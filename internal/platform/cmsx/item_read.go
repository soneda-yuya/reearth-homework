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
// values skip the corresponding query param. Cursor is opaque to the caller
// — it always originates from a previous ListItemsResult.NextCursor.
type ListItemsQuery struct {
	// Filters is a map of CMS field key → desired value. The CMS supports
	// multi-key AND filtering via repeated ?filter[key]=value parameters.
	Filters map[string]string
	// InfoTypes, when non-empty, adds an IN-style filter against the
	// info_type field. Kept separate from Filters so callers don't have to
	// know the comma-separated encoding the CMS expects.
	InfoTypes []string
	// Keyword triggers the CMS text search across title/main_text. Empty
	// skips the search param.
	Keyword string
	Limit   int
	Cursor  string
}

// ListItemsResult is the shape returned from the CMS list endpoint. Items
// are pre-decoded into ItemDTO for convenience. NextCursor is empty when the
// caller has walked the full page.
type ListItemsResult struct {
	Items      []ItemDTO
	NextCursor string
}

// ListItems returns a page of items under modelID matching the query, plus
// an opaque cursor for the next page. The CMS is the single source of truth
// for cursor encoding — we do not attempt to interpret or reconstruct it.
func (c *Client) ListItems(ctx context.Context, modelID string, q ListItemsQuery) (ListItemsResult, error) {
	ctx, span := observability.Tracer(ctx).Start(ctx, "cms.ListItems",
		trace.WithAttributes(
			attribute.String("model.id", modelID),
			attribute.Int("query.limit", q.Limit),
		))
	defer span.End()

	params := url.Values{}
	for k, v := range q.Filters {
		params.Set("filter["+k+"]", v)
	}
	if len(q.InfoTypes) > 0 {
		params.Set("info_types", strings.Join(q.InfoTypes, ","))
	}
	if q.Limit > 0 {
		params.Set("perPage", strconv.Itoa(q.Limit))
	}
	if q.Cursor != "" {
		params.Set("cursor", q.Cursor)
	}

	u := c.url("/api/models/%s/items", modelID)
	if encoded := params.Encode(); encoded != "" {
		u += "?" + encoded
	}

	var out struct {
		Items      []json.RawMessage `json:"items"`
		NextCursor string            `json:"nextCursor"`
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
	return ListItemsResult{Items: items, NextCursor: out.NextCursor}, nil
}

// SearchItems is a thin wrapper around ListItems that moves a keyword into
// the search path. Separate method because keyword search is semantically
// distinct and may diverge (fuzzy matching, relevance ordering) in future.
func (c *Client) SearchItems(ctx context.Context, modelID string, q ListItemsQuery) (ListItemsResult, error) {
	ctx, span := observability.Tracer(ctx).Start(ctx, "cms.SearchItems",
		trace.WithAttributes(
			attribute.String("model.id", modelID),
			attribute.String("query.keyword", q.Keyword),
		))
	defer span.End()

	// Today the CMS supports keyword via ?q=… on the same list endpoint. The
	// wrapper encodes that, keeping callers free of the param name.
	params := url.Values{}
	for k, v := range q.Filters {
		params.Set("filter["+k+"]", v)
	}
	if len(q.InfoTypes) > 0 {
		params.Set("info_types", strings.Join(q.InfoTypes, ","))
	}
	if q.Keyword != "" {
		params.Set("q", q.Keyword)
	}
	if q.Limit > 0 {
		params.Set("perPage", strconv.Itoa(q.Limit))
	}
	if q.Cursor != "" {
		params.Set("cursor", q.Cursor)
	}

	u := c.url("/api/models/%s/items", modelID)
	if encoded := params.Encode(); encoded != "" {
		u += "?" + encoded
	}

	var out struct {
		Items      []json.RawMessage `json:"items"`
		NextCursor string            `json:"nextCursor"`
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
	return ListItemsResult{Items: items, NextCursor: out.NextCursor}, nil
}
