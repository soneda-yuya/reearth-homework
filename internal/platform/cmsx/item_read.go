package cmsx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/soneda-yuya/overseas-safety-map/internal/platform/observability"
)

// ListItemsQuery scopes a ListItems call. Every field is optional; zero
// values skip the corresponding query param.
//
// Cursor is kept on the wire for proto compatibility with the BFF but
// internally encodes the next page number — reearth-cms uses page/perPage
// pagination, not opaque cursors. An empty Cursor starts from page 1.
type ListItemsQuery struct {
	// Filters is a map of CMS field key → desired value. reearth-cms
	// supports a generic keyword search; typed filters are not officially
	// documented on the integration API, so the values are concatenated
	// into a space-separated keyword string as a best-effort filter. The
	// caller is responsible for re-filtering in memory when exactness
	// matters.
	Filters map[string]string
	// InfoTypes, when non-empty, joins the info_type values into the same
	// best-effort keyword. Kept separate so future typed filtering can
	// target the info_type field specifically.
	InfoTypes []string
	// Keyword triggers the CMS text search across title/main_text. Empty
	// skips the search param.
	Keyword string
	Limit   int
	// Cursor: empty means "page 1". Non-empty is the next page number
	// returned by the previous call.
	Cursor string
}

// ListItemsResult is the shape returned from the CMS list endpoint. Items
// are pre-decoded into ItemDTO for convenience. NextCursor is empty when
// the caller has walked the full page set.
type ListItemsResult struct {
	Items      []ItemDTO
	NextCursor string
}

// ListItems returns a page of items under (projectID, modelID) matching the
// query, plus an opaque cursor for the next page. The cursor is actually a
// stringified page number, but we keep the opaque contract so the BFF proto
// does not leak the implementation detail.
func (c *Client) ListItems(ctx context.Context, projectID, modelID string, q ListItemsQuery) (ListItemsResult, error) {
	ctx, span := observability.Tracer(ctx).Start(ctx, "cms.ListItems",
		trace.WithAttributes(
			attribute.String("project.id", projectID),
			attribute.String("model.id", modelID),
			attribute.Int("query.limit", q.Limit),
		))
	defer span.End()

	return c.listItems(ctx, projectID, modelID, q, "")
}

// SearchItems is a thin wrapper that moves a keyword into the search path.
// Separate method because keyword search is semantically distinct and may
// diverge (fuzzy matching, relevance ordering) in future.
func (c *Client) SearchItems(ctx context.Context, projectID, modelID string, q ListItemsQuery) (ListItemsResult, error) {
	ctx, span := observability.Tracer(ctx).Start(ctx, "cms.SearchItems",
		trace.WithAttributes(
			attribute.String("project.id", projectID),
			attribute.String("model.id", modelID),
			attribute.String("query.keyword", q.Keyword),
		))
	defer span.End()

	return c.listItems(ctx, projectID, modelID, q, q.Keyword)
}

func (c *Client) listItems(ctx context.Context, projectID, modelID string, q ListItemsQuery, explicitKeyword string) (ListItemsResult, error) {
	params := url.Values{}

	// Build the keyword from filters + info_types + explicit keyword. reearth-cms
	// integration API does not ship typed filter operators, so we lean on the
	// keyword search (documented, supported) and rely on callers to re-filter
	// in memory for strict equality semantics.
	kwParts := make([]string, 0, len(q.Filters)+len(q.InfoTypes)+1)
	if explicitKeyword != "" {
		kwParts = append(kwParts, explicitKeyword)
	}
	for _, v := range q.Filters {
		if v != "" {
			kwParts = append(kwParts, v)
		}
	}
	for _, v := range q.InfoTypes {
		if v != "" {
			kwParts = append(kwParts, v)
		}
	}
	if kw := strings.Join(kwParts, " "); kw != "" {
		params.Set("keyword", kw)
	}

	if q.Limit > 0 {
		params.Set("perPage", strconv.Itoa(q.Limit))
	}
	// Cursor on the wire is an opaque string; internally it is the page
	// number as a decimal. An empty cursor defaults to page 1.
	page := 1
	if q.Cursor != "" {
		if n, err := strconv.Atoi(q.Cursor); err == nil && n > 0 {
			page = n
		}
	}
	params.Set("page", strconv.Itoa(page))

	u := c.url("/api/%s/projects/%s/models/%s/items", c.cfg.WorkspaceID, projectID, modelID)
	if encoded := params.Encode(); encoded != "" {
		u += "?" + encoded
	}

	var out struct {
		Items      []json.RawMessage `json:"items"`
		Page       int               `json:"page"`
		PerPage    int               `json:"perPage"`
		TotalCount int               `json:"totalCount"`
	}
	if err := c.doJSONRetry(ctx, http.MethodGet, u, nil, &out); err != nil {
		return ListItemsResult{}, err
	}

	items := make([]ItemDTO, 0, len(out.Items))
	for _, raw := range out.Items {
		dto, err := decodeItem(raw)
		if err != nil {
			return ListItemsResult{}, err
		}
		items = append(items, *dto)
	}

	// Synthesise the next cursor: non-empty only when another full page
	// remains. Caller treats the value as opaque and feeds it back on the
	// next call.
	nextCursor := ""
	if out.PerPage > 0 && out.Page*out.PerPage < out.TotalCount {
		nextCursor = strconv.Itoa(out.Page + 1)
	}
	return ListItemsResult{Items: items, NextCursor: nextCursor}, nil
}
