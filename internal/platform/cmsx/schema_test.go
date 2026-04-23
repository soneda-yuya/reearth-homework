package cmsx_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/soneda-yuya/overseas-safety-map/internal/cmsmigrate/domain"
	"github.com/soneda-yuya/overseas-safety-map/internal/platform/cmsx"
	"github.com/soneda-yuya/overseas-safety-map/internal/platform/retry"
	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
)

// fastRetryPolicy is the policy used by tests that exercise the retry path.
// Production uses retry.DefaultPolicy (500ms initial backoff); the unit
// suite injects this faster variant so retry tests do not sleep noticeably.
var fastRetryPolicy = retry.Policy{
	MaxAttempts: 3,
	Initial:     1 * time.Millisecond,
	Max:         10 * time.Millisecond,
	Multiplier:  2.0,
	Jitter:      0,
}

// newClient stands up a test server and a configured client. The returned
// server should be closed by the caller, typically with defer srv.Close().
func newClient(h http.HandlerFunc) (*cmsx.Client, *httptest.Server) {
	srv := httptest.NewServer(h)
	c := cmsx.NewClient(cmsx.Config{
		BaseURL:     srv.URL,
		WorkspaceID: "ws-1",
		Token:       "tok",
		Timeout:     2 * time.Second,
		RetryPolicy: fastRetryPolicy,
	})
	return c, srv
}

func TestFindProjectByAlias_Hit(t *testing.T) {
	t.Parallel()
	c, srv := newClient(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/api/workspaces/ws-1/projects"; got != want {
			t.Errorf("path = %q want %q", got, want)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("auth header = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"id": "p-1", "alias": "other"},
				{"id": "p-2", "alias": "wanted", "name": "W"},
			},
		})
	})
	defer srv.Close()

	got, err := c.FindProjectByAlias(context.Background(), "wanted")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected hit, got nil")
	}
	if got.ID != "p-2" || got.Alias != "wanted" {
		t.Errorf("got %+v", got)
	}
}

func TestFindProjectByAlias_Miss(t *testing.T) {
	t.Parallel()
	c, srv := newClient(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}})
	})
	defer srv.Close()

	got, err := c.FindProjectByAlias(context.Background(), "missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestCreateProject_SendsBody(t *testing.T) {
	t.Parallel()
	var receivedBody map[string]any
	c, srv := newClient(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("content-type = %q", got)
		}
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(b, &receivedBody); err != nil {
			t.Fatalf("unmarshal request body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"p-new","alias":"demo","name":"Demo"}`))
	})
	defer srv.Close()

	got, err := c.CreateProject(context.Background(), domain.ProjectDefinition{
		Alias: "demo", Name: "Demo", Description: "d",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "p-new" {
		t.Errorf("got id %q", got.ID)
	}
	if receivedBody["alias"] != "demo" || receivedBody["name"] != "Demo" || receivedBody["description"] != "d" {
		t.Errorf("body mismatch: %v", receivedBody)
	}
}

func TestCreateProject_Unauthorized_NoRetry(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	c, srv := newClient(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad token"}`))
	})
	defer srv.Close()

	_, err := c.CreateProject(context.Background(), domain.ProjectDefinition{Alias: "x", Name: "X"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errs.IsKind(err, errs.KindUnauthorized) {
		t.Errorf("kind = %s, want %s", errs.KindOf(err), errs.KindUnauthorized)
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("POST should not retry, got %d hits", got)
	}
}

func TestCreateProject_Conflict(t *testing.T) {
	t.Parallel()
	c, srv := newClient(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"exists"}`))
	})
	defer srv.Close()

	_, err := c.CreateProject(context.Background(), domain.ProjectDefinition{Alias: "x", Name: "X"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errs.IsKind(err, errs.KindConflict) {
		t.Errorf("kind = %s, want %s", errs.KindOf(err), errs.KindConflict)
	}
}

func TestFindProjectByAlias_TransientThenSuccess(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	c, srv := newClient(func(w http.ResponseWriter, _ *http.Request) {
		n := hits.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{{"id": "p-1", "alias": "demo"}},
		})
	})
	defer srv.Close()

	got, err := c.FindProjectByAlias(context.Background(), "demo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.Alias != "demo" {
		t.Errorf("got %+v", got)
	}
	if h := hits.Load(); h < 2 {
		t.Errorf("expected retry (>=2 hits), got %d", h)
	}
}

func TestFindFieldByAlias_ParsesSchema(t *testing.T) {
	t.Parallel()
	c, srv := newClient(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if got, want := r.URL.Path, "/api/models/m-1"; got != want {
			t.Errorf("path = %q want %q", got, want)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("auth header = %q", got)
		}
		_, _ = w.Write([]byte(`{
			"id":"m-1","key":"thing","name":"Thing",
			"schema":[
				{"id":"f-a","key":"id","type":"text","required":true,"unique":true},
				{"id":"f-b","key":"body","type":"textArea"}
			]
		}`))
	})
	defer srv.Close()

	got, err := c.FindFieldByAlias(context.Background(), "m-1", "body")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected hit, got nil")
	}
	if got.Type != "textArea" {
		t.Errorf("type = %q want textArea", got.Type)
	}
	if got.ToDomainType() != domain.FieldTypeTextArea {
		t.Errorf("domain type = %s want textArea", got.ToDomainType())
	}

	miss, err := c.FindFieldByAlias(context.Background(), "m-1", "nope")
	if err != nil || miss != nil {
		t.Errorf("expected (nil,nil), got (%+v, %v)", miss, err)
	}
}

// TestRetryPolicy_Injectable ensures Config.RetryPolicy is honoured. With
// MaxAttempts = 1 the client must make exactly one request even if the
// server keeps returning 503. The contract is observed via the hit counter
// (deterministic) rather than wall-clock timing (loaded CI runners).
func TestRetryPolicy_Injectable(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := cmsx.NewClient(cmsx.Config{
		BaseURL:     srv.URL,
		WorkspaceID: "ws-1",
		Token:       "tok",
		Timeout:     2 * time.Second,
		RetryPolicy: retry.Policy{MaxAttempts: 1, Initial: 1 * time.Millisecond, Multiplier: 2.0},
	})

	_, err := c.FindProjectByAlias(context.Background(), "demo")
	if err == nil {
		t.Fatal("expected error from 503")
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("expected exactly 1 attempt with MaxAttempts=1, got %d", got)
	}
}

func TestFindModelByAlias_Hit(t *testing.T) {
	t.Parallel()
	c, srv := newClient(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/api/projects/p-1/models"; got != want {
			t.Errorf("path = %q want %q", got, want)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]any{
				{"id": "m-other", "key": "other"},
				{"id": "m-1", "key": "thing", "name": "Thing"},
			},
		})
	})
	defer srv.Close()

	got, err := c.FindModelByAlias(context.Background(), "p-1", "thing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.ID != "m-1" || got.Alias != "thing" {
		t.Errorf("got %+v", got)
	}
}

func TestFindModelByAlias_Miss(t *testing.T) {
	t.Parallel()
	c, srv := newClient(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"models": []any{}})
	})
	defer srv.Close()

	got, err := c.FindModelByAlias(context.Background(), "p-1", "missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestCreateModel_SendsBody(t *testing.T) {
	t.Parallel()
	var receivedBody map[string]any
	c, srv := newClient(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if got, want := r.URL.Path, "/api/projects/p-1/models"; got != want {
			t.Errorf("path = %q want %q", got, want)
		}
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(b, &receivedBody); err != nil {
			t.Fatalf("unmarshal request body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"m-new","key":"thing","name":"Thing"}`))
	})
	defer srv.Close()

	got, err := c.CreateModel(context.Background(), "p-1", domain.ModelDefinition{
		Alias: "thing", Name: "Thing", Description: "d",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "m-new" || got.Alias != "thing" {
		t.Errorf("got %+v", got)
	}
	// reearth-cms uses "key" for the model alias.
	if receivedBody["key"] != "thing" || receivedBody["name"] != "Thing" || receivedBody["description"] != "d" {
		t.Errorf("body mismatch: %v", receivedBody)
	}
}

func TestCreateModel_NotFoundIsClassified(t *testing.T) {
	t.Parallel()
	c, srv := newClient(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"project not found"}`))
	})
	defer srv.Close()

	_, err := c.CreateModel(context.Background(), "p-x", domain.ModelDefinition{Alias: "x", Name: "X"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errs.IsKind(err, errs.KindNotFound) {
		t.Errorf("kind = %s, want %s", errs.KindOf(err), errs.KindNotFound)
	}
}

func TestCreateField_SerializesType(t *testing.T) {
	t.Parallel()
	var body map[string]any
	c, srv := newClient(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if got, want := r.URL.Path, "/api/models/m-1/fields"; got != want {
			t.Errorf("path = %q want %q", got, want)
		}
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if err := json.Unmarshal(b, &body); err != nil {
			t.Fatalf("unmarshal request body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"f-new","key":"geom","type":"geometryObject"}`))
	})
	defer srv.Close()

	got, err := c.CreateField(context.Background(), "m-1", domain.FieldDefinition{
		Alias: "geom", Name: "Geometry", Type: domain.FieldTypeGeometryObject, Required: false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body["type"] != "geometryObject" || body["key"] != "geom" {
		t.Errorf("body = %v", body)
	}
	if got.Type != "geometryObject" {
		t.Errorf("got.Type = %s", got.Type)
	}
}
