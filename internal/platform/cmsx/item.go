package cmsx

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/soneda-yuya/reearth-homework/internal/platform/observability"
	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

// ItemDTO is the trimmed view of a reearth-cms item we work with from Go.
// Fields is left as a generic map so callers don't need to declare a Go
// struct per Model — the use case knows what keys it cares about.
type ItemDTO struct {
	ID     string                     `json:"id"`
	Fields map[string]any             `json:"fields"`
	raw    map[string]json.RawMessage // captured for inspection if needed
}

// FindItemByFieldValue returns the first item under modelID whose field
// matches value, or (nil, nil) when nothing matches. The CMS Integration
// API supports filtering by field value via query params; we wrap that here
// so the caller doesn't have to assemble the URL.
func (c *Client) FindItemByFieldValue(ctx context.Context, modelID, fieldKey, value string) (*ItemDTO, error) {
	ctx, span := observability.Tracer(ctx).Start(ctx, "cms.FindItemByFieldValue",
		trace.WithAttributes(
			attribute.String("model.id", modelID),
			attribute.String("field.key", fieldKey),
		))
	defer span.End()

	// Path components must be PathEscape-d, but query parameters need
	// QueryEscape semantics (notably so '+' encodes correctly). Build the
	// two halves separately and concatenate.
	q := url.Values{}
	q.Set("key", fieldKey)
	q.Set("value", value)
	u := c.url("/api/models/%s/items", modelID) + "?" + q.Encode()

	var out struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := c.doJSONRetry(ctx, http.MethodGet, u, nil, &out); err != nil {
		// 404 is the documented "no item matches" response on some CMS
		// versions — collapse it into the (nil, nil) miss path so callers
		// don't have to special-case it.
		if errs.IsKind(err, errs.KindNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if len(out.Items) == 0 {
		return nil, nil
	}
	return decodeItem(out.Items[0])
}

// CreateItem POSTs a fresh item with the given field map. The response is
// parsed into an ItemDTO so the caller can chain follow-up calls (rare in
// U-ING — Pub/Sub publish doesn't need the new ID).
func (c *Client) CreateItem(ctx context.Context, modelID string, fields map[string]any) (*ItemDTO, error) {
	ctx, span := observability.Tracer(ctx).Start(ctx, "cms.CreateItem",
		trace.WithAttributes(attribute.String("model.id", modelID)))
	defer span.End()

	body := struct {
		Fields map[string]any `json:"fields"`
	}{Fields: fields}

	u := c.url("/api/models/%s/items", modelID)
	var raw json.RawMessage
	if err := c.doJSON(ctx, http.MethodPost, u, body, &raw); err != nil {
		return nil, err
	}
	return decodeItem(raw)
}

// UpdateItem PATCHes an existing item with new field values. The CMS API
// accepts partial updates; only fields present in the map are touched.
func (c *Client) UpdateItem(ctx context.Context, itemID string, fields map[string]any) (*ItemDTO, error) {
	ctx, span := observability.Tracer(ctx).Start(ctx, "cms.UpdateItem",
		trace.WithAttributes(attribute.String("item.id", itemID)))
	defer span.End()

	body := struct {
		Fields map[string]any `json:"fields"`
	}{Fields: fields}

	u := c.url("/api/items/%s", itemID)
	var raw json.RawMessage
	if err := c.doJSON(ctx, http.MethodPatch, u, body, &raw); err != nil {
		return nil, err
	}
	return decodeItem(raw)
}

// UpsertItemByFieldValue is the convenience the use case actually wants:
// look up by (modelID, fieldKey, value); update if found, otherwise create.
// The caller is expected to include fieldKey/value inside the fields map so
// new items carry the unique-key identity.
func (c *Client) UpsertItemByFieldValue(ctx context.Context, modelID, fieldKey, value string, fields map[string]any) (*ItemDTO, error) {
	existing, err := c.FindItemByFieldValue(ctx, modelID, fieldKey, value)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return c.UpdateItem(ctx, existing.ID, fields)
	}
	return c.CreateItem(ctx, modelID, fields)
}

// decodeItem unmarshals a raw JSON object into ItemDTO. Unknown fields are
// preserved in raw so callers can introspect later if needed.
func decodeItem(raw json.RawMessage) (*ItemDTO, error) {
	var shape struct {
		ID     string         `json:"id"`
		Fields map[string]any `json:"fields"`
	}
	if err := json.Unmarshal(raw, &shape); err != nil {
		return nil, errs.Wrap("cmsx.decode_item", errs.KindInternal, err)
	}
	if shape.Fields == nil {
		shape.Fields = map[string]any{}
	}
	dto := &ItemDTO{ID: shape.ID, Fields: shape.Fields}

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
