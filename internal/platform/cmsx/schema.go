package cmsx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/soneda-yuya/reearth-homework/internal/cmsmigrate/domain"
	"github.com/soneda-yuya/reearth-homework/internal/platform/observability"
	"github.com/soneda-yuya/reearth-homework/internal/platform/retry"
	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

// FindProjectByAlias returns the project identified by alias, or (nil, nil)
// if the CMS does not have one. The LIST-then-filter approach tolerates
// small CMS payloads; pagination lands when we actually have many projects.
func (c *Client) FindProjectByAlias(ctx context.Context, alias string) (*ProjectDTO, error) {
	ctx, span := observability.Tracer(ctx).Start(ctx, "cms.FindProjectByAlias",
		trace.WithAttributes(attribute.String("project.alias", alias)))
	defer span.End()

	u := c.url("/api/workspaces/%s/projects", c.cfg.WorkspaceID)
	var out struct {
		Items []ProjectDTO `json:"items"`
	}
	if err := c.doJSONRetry(ctx, http.MethodGet, u, nil, &out); err != nil {
		return nil, err
	}
	for i := range out.Items {
		if out.Items[i].Alias == alias {
			return &out.Items[i], nil
		}
	}
	return nil, nil
}

// CreateProject creates a project under the configured workspace.
func (c *Client) CreateProject(ctx context.Context, def domain.ProjectDefinition) (*ProjectDTO, error) {
	ctx, span := observability.Tracer(ctx).Start(ctx, "cms.CreateProject",
		trace.WithAttributes(attribute.String("project.alias", def.Alias)))
	defer span.End()

	body := createProjectBody{Alias: def.Alias, Name: def.Name, Description: def.Description}
	u := c.url("/api/workspaces/%s/projects", c.cfg.WorkspaceID)
	var out ProjectDTO
	if err := c.doJSON(ctx, http.MethodPost, u, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// FindModelByAlias returns the model for (projectID, alias), or (nil, nil)
// when missing.
func (c *Client) FindModelByAlias(ctx context.Context, projectID, alias string) (*ModelDTO, error) {
	ctx, span := observability.Tracer(ctx).Start(ctx, "cms.FindModelByAlias",
		trace.WithAttributes(attribute.String("model.alias", alias)))
	defer span.End()

	u := c.url("/api/projects/%s/models", projectID)
	var out struct {
		Models []ModelDTO `json:"models"`
	}
	if err := c.doJSONRetry(ctx, http.MethodGet, u, nil, &out); err != nil {
		return nil, err
	}
	for i := range out.Models {
		if out.Models[i].Alias == alias {
			return &out.Models[i], nil
		}
	}
	return nil, nil
}

// CreateModel creates a model under the given project.
func (c *Client) CreateModel(ctx context.Context, projectID string, def domain.ModelDefinition) (*ModelDTO, error) {
	ctx, span := observability.Tracer(ctx).Start(ctx, "cms.CreateModel",
		trace.WithAttributes(attribute.String("model.alias", def.Alias)))
	defer span.End()

	body := createModelBody{Alias: def.Alias, Name: def.Name, Description: def.Description}
	u := c.url("/api/projects/%s/models", projectID)
	var out ModelDTO
	if err := c.doJSON(ctx, http.MethodPost, u, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// FindFieldByAlias returns the field for (modelID, alias), or (nil, nil)
// when missing.
func (c *Client) FindFieldByAlias(ctx context.Context, modelID, alias string) (*FieldDTO, error) {
	ctx, span := observability.Tracer(ctx).Start(ctx, "cms.FindFieldByAlias",
		trace.WithAttributes(attribute.String("field.alias", alias)))
	defer span.End()

	u := c.url("/api/models/%s", modelID)
	var out ModelDTO
	if err := c.doJSONRetry(ctx, http.MethodGet, u, nil, &out); err != nil {
		return nil, err
	}
	for i := range out.Fields {
		if out.Fields[i].Alias == alias {
			return &out.Fields[i], nil
		}
	}
	return nil, nil
}

// CreateField creates a field on the given model.
func (c *Client) CreateField(ctx context.Context, modelID string, def domain.FieldDefinition) (*FieldDTO, error) {
	ctx, span := observability.Tracer(ctx).Start(ctx, "cms.CreateField",
		trace.WithAttributes(attribute.String("field.alias", def.Alias)))
	defer span.End()

	body := createFieldBody{
		Alias:       def.Alias,
		Name:        def.Name,
		Description: def.Description,
		Type:        fieldTypeToAPI(def.Type),
		Required:    def.Required,
		Unique:      def.Unique,
		Multiple:    def.Multiple,
	}
	u := c.url("/api/models/%s/fields", modelID)
	var out FieldDTO
	if err := c.doJSON(ctx, http.MethodPost, u, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// --- transport -------------------------------------------------------------

// url builds an absolute URL under cfg.BaseURL. Path components are escaped
// to avoid injection via workspace or project IDs.
func (c *Client) url(pathFmt string, args ...string) string {
	escaped := make([]any, len(args))
	for i, a := range args {
		escaped[i] = url.PathEscape(a)
	}
	return c.cfg.BaseURL + fmt.Sprintf(pathFmt, escaped...)
}

// doJSON issues one HTTP call with Bearer auth. CREATE paths live here
// because POST without an idempotency key must not be retried on 5xx.
func (c *Client) doJSON(ctx context.Context, method, u string, in, out any) error {
	return c.doOnce(ctx, method, u, in, out)
}

// doJSONRetry is for idempotent reads (GET). Transient 5xx / 429 are retried
// via retry.Do with the default policy (3 attempts, 500ms initial).
func (c *Client) doJSONRetry(ctx context.Context, method, u string, in, out any) error {
	return retry.Do(ctx, retry.DefaultPolicy, func(ctx context.Context) error {
		return c.doOnce(ctx, method, u, in, out)
	})
}

func (c *Client) doOnce(ctx context.Context, method, u string, in, out any) error {
	var body io.Reader
	if in != nil {
		buf, err := json.Marshal(in)
		if err != nil {
			return errs.Wrap("cmsx.marshal", errs.KindInternal, err)
		}
		body = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return errs.Wrap("cmsx.newRequest", errs.KindInternal, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		// Network-level failure (DNS, connection reset, context) is transient.
		return errs.Wrap("cmsx.http", errs.KindExternal, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	apiErr := &apiError{method: method, url: u, status: resp.StatusCode, body: string(respBody)}

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		if out == nil {
			return nil
		}
		if err := json.Unmarshal(respBody, out); err != nil {
			return errs.Wrap("cmsx.decode", errs.KindInternal, err)
		}
		return nil
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		return errs.Wrap("cmsx.auth", errs.KindUnauthorized, apiErr)
	case resp.StatusCode == http.StatusConflict:
		return errs.Wrap("cmsx.conflict", errs.KindConflict, apiErr)
	case resp.StatusCode == http.StatusNotFound:
		// Callers that use "FindByAlias" shouldn't hit 404 for a whole
		// resource; mapping as KindNotFound gives the caller the option to
		// treat it as "missing" without guessing.
		return errs.Wrap("cmsx.not_found", errs.KindNotFound, apiErr)
	case resp.StatusCode == http.StatusTooManyRequests, resp.StatusCode >= 500:
		return errs.Wrap("cmsx.transient", errs.KindExternal, apiErr)
	default:
		return errs.Wrap("cmsx.unexpected", errs.KindInternal, apiErr)
	}
}

// ErrConflict is provided for callers that want to branch on CMS-side
// conflicts without importing errs. It is intentionally small; most callers
// should use errs.IsKind(err, errs.KindConflict) directly.
var ErrConflict = errors.New("cmsx: conflict")

// ToRemoteField adapts a FieldDTO into the application-layer RemoteField
// shape. Exported so the cmsclient adapter does not re-implement the type
// translation.
func (d FieldDTO) ToDomainType() domain.FieldType {
	return fieldTypeFromAPI(d.Type)
}
