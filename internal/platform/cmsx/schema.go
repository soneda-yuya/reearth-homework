package cmsx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/soneda-yuya/overseas-safety-map/internal/cmsmigrate/domain"
	"github.com/soneda-yuya/overseas-safety-map/internal/platform/observability"
	"github.com/soneda-yuya/overseas-safety-map/internal/platform/retry"
	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
)

// FindProjectByAlias returns the project identified by alias within the
// configured workspace, or (nil, nil) if no such project exists. The LIST
// endpoint is paginated on the CMS side (page/perPage), but project counts
// per workspace are tiny so we rely on the default first page. Pagination
// lands if we ever have more than ~50 projects per workspace.
func (c *Client) FindProjectByAlias(ctx context.Context, alias string) (*ProjectDTO, error) {
	ctx, span := observability.Tracer(ctx).Start(ctx, "cms.FindProjectByAlias",
		trace.WithAttributes(attribute.String("project.alias", alias)))
	defer span.End()

	u := c.url("/api/%s/projects", c.cfg.WorkspaceID)
	var out struct {
		Projects []ProjectDTO `json:"projects"`
	}
	if err := c.doJSONRetry(ctx, http.MethodGet, u, nil, &out); err != nil {
		return nil, err
	}
	for i := range out.Projects {
		if out.Projects[i].Alias == alias {
			return &out.Projects[i], nil
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
	u := c.url("/api/%s/projects", c.cfg.WorkspaceID)
	var out ProjectDTO
	if err := c.doJSON(ctx, http.MethodPost, u, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// FindModelByAlias returns the model under (projectIdOrAlias, alias), or
// (nil, nil) when missing. The projectIdOrAlias parameter accepts either
// form — reearth-cms resolves both in the path.
func (c *Client) FindModelByAlias(ctx context.Context, projectIDOrAlias, alias string) (*ModelDTO, error) {
	ctx, span := observability.Tracer(ctx).Start(ctx, "cms.FindModelByAlias",
		trace.WithAttributes(attribute.String("model.alias", alias)))
	defer span.End()

	u := c.url("/api/%s/projects/%s/models", c.cfg.WorkspaceID, projectIDOrAlias)
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

// CreateModel creates a model under the given project. The returned ModelDTO
// carries SchemaID, which callers must pass to CreateField for this model's
// fields.
func (c *Client) CreateModel(ctx context.Context, projectIDOrAlias string, def domain.ModelDefinition) (*ModelDTO, error) {
	ctx, span := observability.Tracer(ctx).Start(ctx, "cms.CreateModel",
		trace.WithAttributes(attribute.String("model.alias", def.Alias)))
	defer span.End()

	body := createModelBody{Alias: def.Alias, Name: def.Name, Description: def.Description}
	u := c.url("/api/%s/projects/%s/models", c.cfg.WorkspaceID, projectIDOrAlias)
	var out ModelDTO
	if err := c.doJSON(ctx, http.MethodPost, u, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// FindFieldByAlias returns the field under the given model, or (nil, nil)
// when missing. reearth-cms nests fields inside the model's schema object,
// so we GET the model and scan the decoded Fields.
func (c *Client) FindFieldByAlias(ctx context.Context, projectIDOrAlias, modelIDOrKey, alias string) (*FieldDTO, error) {
	ctx, span := observability.Tracer(ctx).Start(ctx, "cms.FindFieldByAlias",
		trace.WithAttributes(attribute.String("field.alias", alias)))
	defer span.End()

	u := c.url("/api/%s/projects/%s/models/%s", c.cfg.WorkspaceID, projectIDOrAlias, modelIDOrKey)
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

// CreateField creates a field on the given schema. Unlike the previous CMS
// API hypothesis, fields belong to a schema (not directly to a model); the
// schemaId comes from the ModelDTO returned by CreateModel / FindModelByAlias.
func (c *Client) CreateField(ctx context.Context, projectIDOrAlias, schemaID string, def domain.FieldDefinition) (*FieldDTO, error) {
	ctx, span := observability.Tracer(ctx).Start(ctx, "cms.CreateField",
		trace.WithAttributes(attribute.String("field.alias", def.Alias)))
	defer span.End()

	body := createFieldBody{
		Type:     fieldTypeToAPI(def.Type),
		Key:      def.Alias,
		Required: def.Required,
		Multiple: def.Multiple,
	}
	u := c.url("/api/%s/projects/%s/schemata/%s/fields", c.cfg.WorkspaceID, projectIDOrAlias, schemaID)
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

// doJSONRetry is for idempotent reads. Transient 5xx / 429 are retried via
// retry.Do with the configured policy. The method must be GET so that a
// future caller cannot accidentally retry a non-idempotent verb.
func (c *Client) doJSONRetry(ctx context.Context, method, u string, in, out any) error {
	if method != http.MethodGet {
		return errs.Wrap("cmsx.retry_method", errs.KindInternal,
			fmt.Errorf("doJSONRetry only supports %s, got %s", http.MethodGet, method))
	}
	return retry.Do(ctx, c.cfg.RetryPolicy, func(ctx context.Context) error {
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

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		// Body read failures are network-level (truncated transfer, peer
		// reset mid-stream); classify as transient so GET retries can pick
		// up but POST surfaces immediately as the original error.
		return errs.Wrap("cmsx.read_body", errs.KindExternal, err)
	}
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
