package cmsx_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/soneda-yuya/overseas-safety-map/internal/platform/cmsx"
)

// TestListItems_BuildsQueryAndDecodes exercises the new reearth-cms contract:
// filters + info_types collapse into a keyword param (typed filters are not
// available on the integration API), and pagination is page/perPage with a
// synthesised "next page" cursor.
func TestListItems_BuildsQueryAndDecodes(t *testing.T) {
	t.Parallel()
	c, srv := newClient(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/api/ws-1/projects/p-1/models/m-1/items"; got != want {
			t.Errorf("path = %q want %q", got, want)
		}
		q := r.URL.Query()
		keyword := q.Get("keyword")
		// Assertions are order-independent because the filter map iteration
		// order is not stable. Require both filter value and info_types to
		// have been folded into the keyword blob.
		if !strings.Contains(keyword, "JP") ||
			!strings.Contains(keyword, "spot_info") ||
			!strings.Contains(keyword, "warning") {
			t.Errorf("keyword = %q; must contain JP, spot_info, warning", keyword)
		}
		if got := q.Get("perPage"); got != "50" {
			t.Errorf("perPage = %q", got)
		}
		// Caller passed Cursor="2" → client sends page=2.
		if got := q.Get("page"); got != "2" {
			t.Errorf("page = %q; want 2", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []json.RawMessage{
				json.RawMessage(`{"id":"i-1","fields":[{"key":"key_cd","type":"text","value":"K1"}]}`),
				json.RawMessage(`{"id":"i-2","fields":[{"key":"key_cd","type":"text","value":"K2"}]}`),
			},
			// Page 2 of 50 with 120 total means another page (page 3) is still
			// available; the client synthesises NextCursor="3".
			"page":       2,
			"perPage":    50,
			"totalCount": 120,
		})
	})
	defer srv.Close()

	res, err := c.ListItems(context.Background(), "p-1", "m-1", cmsx.ListItemsQuery{
		Filters:   map[string]string{"country_cd": "JP"},
		InfoTypes: []string{"spot_info", "warning"},
		Limit:     50,
		Cursor:    "2",
	})
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(res.Items) != 2 {
		t.Errorf("len(Items) = %d", len(res.Items))
	}
	if res.NextCursor != "3" {
		t.Errorf("NextCursor = %q; want 3", res.NextCursor)
	}
}

func TestListItems_EmptyResponse(t *testing.T) {
	t.Parallel()
	c, srv := newClient(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}, "page": 1, "perPage": 0, "totalCount": 0})
	})
	defer srv.Close()

	res, err := c.ListItems(context.Background(), "p-1", "m-1", cmsx.ListItemsQuery{})
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(res.Items) != 0 {
		t.Errorf("len(Items) = %d; want 0", len(res.Items))
	}
	if res.NextCursor != "" {
		t.Errorf("NextCursor = %q; want empty", res.NextCursor)
	}
}

// TestSearchItems_PassesKeyword guards the SearchItems wrapper. The keyword
// supplied by the caller must reach the CMS via the `keyword` query param
// (reearth-cms's documented full-text search).
func TestSearchItems_PassesKeyword(t *testing.T) {
	t.Parallel()
	c, srv := newClient(func(w http.ResponseWriter, r *http.Request) {
		got := r.URL.Query().Get("keyword")
		if !strings.Contains(got, "earthquake") {
			t.Errorf("keyword = %q; must contain earthquake", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}, "page": 1, "perPage": 0, "totalCount": 0})
	})
	defer srv.Close()

	if _, err := c.SearchItems(context.Background(), "p-1", "m-1", cmsx.ListItemsQuery{Keyword: "earthquake"}); err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
}
