package cmsx_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/soneda-yuya/overseas-safety-map/internal/platform/cmsx"
)

func TestListItems_BuildsQueryAndDecodes(t *testing.T) {
	t.Parallel()
	c, srv := newClient(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if got := q.Get("filter[country_cd]"); got != "JP" {
			t.Errorf("filter[country_cd] = %q", got)
		}
		if got := q.Get("info_types"); got != "spot_info,warning" {
			t.Errorf("info_types = %q", got)
		}
		if got := q.Get("perPage"); got != "50" {
			t.Errorf("perPage = %q", got)
		}
		if got := q.Get("cursor"); got != "prev-token" {
			t.Errorf("cursor = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"id": "i-1", "fields": map[string]any{"key_cd": "K1"}},
				{"id": "i-2", "fields": map[string]any{"key_cd": "K2"}},
			},
			"nextCursor": "next-token",
		})
	})
	defer srv.Close()

	res, err := c.ListItems(context.Background(), "m-1", cmsx.ListItemsQuery{
		Filters:   map[string]string{"country_cd": "JP"},
		InfoTypes: []string{"spot_info", "warning"},
		Limit:     50,
		Cursor:    "prev-token",
	})
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(res.Items) != 2 {
		t.Errorf("len(Items) = %d", len(res.Items))
	}
	if res.NextCursor != "next-token" {
		t.Errorf("NextCursor = %q", res.NextCursor)
	}
}

func TestListItems_EmptyResponse(t *testing.T) {
	t.Parallel()
	c, srv := newClient(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}})
	})
	defer srv.Close()

	res, err := c.ListItems(context.Background(), "m-1", cmsx.ListItemsQuery{})
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

func TestSearchItems_PassesKeyword(t *testing.T) {
	t.Parallel()
	c, srv := newClient(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("q"); got != "earthquake" {
			t.Errorf("q = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}})
	})
	defer srv.Close()

	if _, err := c.SearchItems(context.Background(), "m-1", cmsx.ListItemsQuery{Keyword: "earthquake"}); err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
}
