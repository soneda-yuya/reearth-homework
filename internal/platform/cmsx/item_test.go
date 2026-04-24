package cmsx_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
)

// Item responses now embed a typed array; canned responses reflect the real
// reearth-cms wire shape: `fields: [{id?,key,type,value}]`.
const itemIResp = `{"id":"i-1","fields":[{"key":"key_cd","type":"text","value":"X"}]}`

func TestFindItemByFieldValue_Hit(t *testing.T) {
	t.Parallel()
	c, srv := newClient(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/api/ws-1/projects/p-1/models/m-1/items"; got != want {
			t.Errorf("path = %q want %q", got, want)
		}
		if got := r.URL.Query().Get("keyword"); got != "X" {
			t.Errorf("keyword query = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []json.RawMessage{json.RawMessage(itemIResp)},
		})
	})
	defer srv.Close()

	got, err := c.FindItemByFieldValue(context.Background(), "p-1", "m-1", "key_cd", "X")
	if err != nil {
		t.Fatalf("FindItemByFieldValue: %v", err)
	}
	if got == nil || got.ID != "i-1" {
		t.Errorf("got %+v, want id=i-1", got)
	}
	if v, _ := got.Fields["key_cd"].(string); v != "X" {
		t.Errorf("decoded field key_cd = %v, want X", got.Fields["key_cd"])
	}
}

// TestFindItemByFieldValue_QueryEscapesSpecialChars guards against a past
// bug where the URL was built via PathEscape, so '+' / '&' / '=' in field
// values would be mis-decoded by the server. We assert the server sees the
// exact value the client passed in.
func TestFindItemByFieldValue_QueryEscapesSpecialChars(t *testing.T) {
	t.Parallel()
	const trickyValue = "MOFA+2026 04/23&id=001"
	c, srv := newClient(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("keyword"); got != trickyValue {
			t.Errorf("keyword query = %q, want %q", got, trickyValue)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}})
	})
	defer srv.Close()

	if _, err := c.FindItemByFieldValue(context.Background(), "p-1", "m-1", "key_cd", trickyValue); err != nil {
		t.Fatalf("FindItemByFieldValue: %v", err)
	}
}

func TestFindItemByFieldValue_Miss_EmptyArray(t *testing.T) {
	t.Parallel()
	c, srv := newClient(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}})
	})
	defer srv.Close()

	got, err := c.FindItemByFieldValue(context.Background(), "p-1", "m-1", "key_cd", "missing")
	if err != nil {
		t.Fatalf("FindItemByFieldValue: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestFindItemByFieldValue_404IsTreatedAsMiss(t *testing.T) {
	t.Parallel()
	c, srv := newClient(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"no item"}`))
	})
	defer srv.Close()

	got, err := c.FindItemByFieldValue(context.Background(), "p-1", "m-1", "key_cd", "missing")
	if err != nil {
		t.Fatalf("FindItemByFieldValue should swallow 404: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for 404 miss")
	}
}

func TestCreateItem_PostsFields(t *testing.T) {
	t.Parallel()
	var posted map[string]any
	c, srv := newClient(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/api/ws-1/projects/p-1/models/m-1/items") {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&posted)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"i-new","fields":[{"key":"key_cd","type":"text","value":"X"}]}`))
	})
	defer srv.Close()

	got, err := c.CreateItem(context.Background(), "p-1", "m-1", map[string]any{"key_cd": "X", "title": "T"})
	if err != nil {
		t.Fatalf("CreateItem: %v", err)
	}
	if got.ID != "i-new" {
		t.Errorf("id = %q", got.ID)
	}
	fields, _ := posted["fields"].([]any)
	if len(fields) != 2 {
		t.Fatalf("posted fields len = %d, want 2 (%v)", len(fields), posted)
	}
	// Each entry carries key/type/value; order is map-iteration-dependent so
	// assert via a set check.
	keys := map[string]bool{}
	for _, f := range fields {
		m, _ := f.(map[string]any)
		keys[m["key"].(string)] = true
		if m["type"] != "text" {
			t.Errorf("type = %v, want text", m["type"])
		}
	}
	if !keys["key_cd"] || !keys["title"] {
		t.Errorf("keys = %v, want both key_cd and title", keys)
	}
}

func TestUpdateItem_PatchesItem(t *testing.T) {
	t.Parallel()
	c, srv := newClient(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %s, want PATCH", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/api/ws-1/projects/p-1/models/m-1/items/i-1") {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":"i-1","fields":[{"key":"title","type":"text","value":"updated"}]}`))
	})
	defer srv.Close()

	got, err := c.UpdateItem(context.Background(), "p-1", "m-1", "i-1", map[string]any{"title": "updated"})
	if err != nil {
		t.Fatalf("UpdateItem: %v", err)
	}
	if got.ID != "i-1" {
		t.Errorf("id = %q", got.ID)
	}
}

func TestUpsertItemByFieldValue_CreatesWhenMissing(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	c, srv := newClient(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}})
		case http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"i-new","fields":[{"key":"key_cd","type":"text","value":"X"}]}`))
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	})
	defer srv.Close()

	got, err := c.UpsertItemByFieldValue(context.Background(), "p-1", "m-1", "key_cd", "X",
		map[string]any{"key_cd": "X", "title": "T"})
	if err != nil {
		t.Fatalf("UpsertItemByFieldValue: %v", err)
	}
	if got.ID != "i-new" {
		t.Errorf("id = %q, want i-new", got.ID)
	}
	if h := hits.Load(); h != 2 {
		t.Errorf("expected GET + POST (2 hits), got %d", h)
	}
}

func TestUpsertItemByFieldValue_UpdatesWhenFound(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	c, srv := newClient(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []json.RawMessage{json.RawMessage(`{"id":"i-existing","fields":[{"key":"key_cd","type":"text","value":"X"}]}`)},
			})
		case http.MethodPatch:
			_, _ = w.Write([]byte(`{"id":"i-existing","fields":[{"key":"title","type":"text","value":"T2"}]}`))
		default:
			t.Errorf("unexpected method %s", r.Method)
		}
	})
	defer srv.Close()

	got, err := c.UpsertItemByFieldValue(context.Background(), "p-1", "m-1", "key_cd", "X",
		map[string]any{"title": "T2"})
	if err != nil {
		t.Fatalf("UpsertItemByFieldValue: %v", err)
	}
	if got.ID != "i-existing" {
		t.Errorf("id = %q, want i-existing", got.ID)
	}
	if h := hits.Load(); h != 2 {
		t.Errorf("expected GET + PATCH (2 hits), got %d", h)
	}
}

func TestCreateItem_AuthFailureNoRetry(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	c, srv := newClient(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	})
	defer srv.Close()

	_, err := c.CreateItem(context.Background(), "p-1", "m-1", map[string]any{"key_cd": "X"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errs.IsKind(err, errs.KindUnauthorized) {
		t.Errorf("kind = %s, want KindUnauthorized", errs.KindOf(err))
	}
	if h := hits.Load(); h != 1 {
		t.Errorf("POST should not retry, got %d hits", h)
	}
}
