package cmsx

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/soneda-yuya/overseas-safety-map/internal/platform/observability"
	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
)

// ItemDTO is the trimmed view of a reearth-cms item we work with from Go.
// Fields is kept as a flat map[key]value for caller convenience; the wire
// form is a typed array ([{id, type, value, key}]) that ItemDTO internally
// flattens on decode and re-expands on encode.
type ItemDTO struct {
	ID     string
	Fields map[string]any
	raw    map[string]json.RawMessage
}

// itemFieldWire is a single element of the CMS item "fields" array.
type itemFieldWire struct {
	ID    string `json:"id,omitempty"`
	Key   string `json:"key"`
	Type  string `json:"type"`
	Value any    `json:"value"`
}

// FindItemByFieldValue returns the first item under (projectID, modelID)
// whose field matches value, or (nil, nil) when nothing matches. The
// Integration API does not expose a true "find by field" endpoint, so we
// LIST with a keyword filter and pick the first match.
func (c *Client) FindItemByFieldValue(ctx context.Context, projectID, modelID, fieldKey, value string) (*ItemDTO, error) {
	ctx, span := observability.Tracer(ctx).Start(ctx, "cms.FindItemByFieldValue",
		trace.WithAttributes(
			attribute.String("project.id", projectID),
			attribute.String("model.id", modelID),
			attribute.String("field.key", fieldKey),
		))
	defer span.End()

	q := url.Values{}
	q.Set("keyword", value)
	u := c.url("/api/%s/projects/%s/models/%s/items", c.cfg.WorkspaceID, projectID, modelID) + "?" + q.Encode()

	var out struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := c.doJSONRetry(ctx, http.MethodGet, u, nil, &out); err != nil {
		if errs.IsKind(err, errs.KindNotFound) {
			return nil, nil
		}
		return nil, err
	}
	for _, raw := range out.Items {
		dto, err := decodeItem(raw)
		if err != nil {
			return nil, err
		}
		if got, _ := dto.Fields[fieldKey].(string); got == value {
			return dto, nil
		}
	}
	return nil, nil
}

// CreateItem POSTs a new item with the given field map. The map is converted
// into the typed array shape reearth-cms requires via inferFieldType.
func (c *Client) CreateItem(ctx context.Context, projectID, modelID string, fields map[string]any) (*ItemDTO, error) {
	ctx, span := observability.Tracer(ctx).Start(ctx, "cms.CreateItem",
		trace.WithAttributes(
			attribute.String("project.id", projectID),
			attribute.String("model.id", modelID),
		))
	defer span.End()

	body := struct {
		Fields []itemFieldWire `json:"fields"`
	}{Fields: fieldsMapToWire(fields)}

	u := c.url("/api/%s/projects/%s/models/%s/items", c.cfg.WorkspaceID, projectID, modelID)
	var raw json.RawMessage
	if err := c.doJSON(ctx, http.MethodPost, u, body, &raw); err != nil {
		return nil, err
	}
	return decodeItem(raw)
}

// UpdateItem PATCHes an existing item with new field values. Unlike the
// previous API hypothesis, reearth-cms requires modelId on the update path.
func (c *Client) UpdateItem(ctx context.Context, projectID, modelID, itemID string, fields map[string]any) (*ItemDTO, error) {
	ctx, span := observability.Tracer(ctx).Start(ctx, "cms.UpdateItem",
		trace.WithAttributes(
			attribute.String("project.id", projectID),
			attribute.String("model.id", modelID),
			attribute.String("item.id", itemID),
		))
	defer span.End()

	body := struct {
		Fields []itemFieldWire `json:"fields"`
	}{Fields: fieldsMapToWire(fields)}

	u := c.url("/api/%s/projects/%s/models/%s/items/%s", c.cfg.WorkspaceID, projectID, modelID, itemID)
	var raw json.RawMessage
	if err := c.doJSON(ctx, http.MethodPatch, u, body, &raw); err != nil {
		return nil, err
	}
	return decodeItem(raw)
}

// UpsertItemByFieldValue is the convenience the use case wants: look up by
// (projectID, modelID, fieldKey, value); update if found, otherwise create.
// The caller is expected to include fieldKey/value inside the fields map so
// new items carry the unique-key identity.
func (c *Client) UpsertItemByFieldValue(ctx context.Context, projectID, modelID, fieldKey, value string, fields map[string]any) (*ItemDTO, error) {
	existing, err := c.FindItemByFieldValue(ctx, projectID, modelID, fieldKey, value)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return c.UpdateItem(ctx, projectID, modelID, existing.ID, fields)
	}
	return c.CreateItem(ctx, projectID, modelID, fields)
}

// fieldsMapToWire converts a flat field map into the typed array shape
// reearth-cms expects. Types are inferred from the Go value; see
// inferFieldType for the rules. Zero-value entries are skipped so we do not
// clobber server-side values with empty strings.
func fieldsMapToWire(fields map[string]any) []itemFieldWire {
	out := make([]itemFieldWire, 0, len(fields))
	for k, v := range fields {
		if isZero(v) {
			continue
		}
		out = append(out, itemFieldWire{Key: k, Type: inferFieldType(v), Value: v})
	}
	return out
}

var rfc3339Re = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$`)

// inferFieldType maps a Go value onto a reearth-cms field type string. The
// ruleset covers exactly the types the MVP domain uses; unknown shapes fall
// back to "text" and let the server surface the mismatch at write time.
func inferFieldType(v any) string {
	switch t := v.(type) {
	case map[string]any:
		return "geometryObject"
	case string:
		if rfc3339Re.MatchString(t) {
			return "date"
		}
		return "text"
	case bool:
		return "bool"
	case int, int64, int32:
		return "integer"
	case float32, float64:
		return "number"
	default:
		return "text"
	}
}

// isZero detects values that should be omitted from the wire payload. Empty
// strings in particular show up whenever the domain left an optional field
// blank (lead, info_url, etc.); sending them would overwrite any CMS-side
// value with "", which is semantically different from "unchanged".
func isZero(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return t == ""
	case map[string]any:
		return len(t) == 0
	default:
		return false
	}
}

// decodeItem unmarshals a raw JSON object into ItemDTO. The wire "fields" is
// an array of typed entries; we flatten it to the caller-friendly map shape
// keyed by field alias. Unknown fields are preserved in raw for inspection.
func decodeItem(raw json.RawMessage) (*ItemDTO, error) {
	var shape struct {
		ID     string          `json:"id"`
		Fields []itemFieldWire `json:"fields"`
	}
	if err := json.Unmarshal(raw, &shape); err != nil {
		return nil, errs.Wrap("cmsx.decode_item", errs.KindInternal, err)
	}
	dto := &ItemDTO{ID: shape.ID, Fields: make(map[string]any, len(shape.Fields))}
	for _, f := range shape.Fields {
		dto.Fields[f.Key] = f.Value
	}

	var bag map[string]json.RawMessage
	if err := json.Unmarshal(raw, &bag); err == nil {
		dto.raw = bag
	}
	if dto.ID == "" {
		return nil, errs.Wrap("cmsx.decode_item",
			errs.KindInternal,
			fmt.Errorf("item missing id"))
	}
	return dto, nil
}
