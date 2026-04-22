package cmsx_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/soneda-yuya/reearth-homework/internal/cmsmigrate/domain"
	"github.com/soneda-yuya/reearth-homework/internal/platform/cmsx"
	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

// newClient stands up a test server + configured client. The returned
// teardown func must be deferred.
func newClient(h http.HandlerFunc) (*cmsx.Client, *httptest.Server) {
	srv := httptest.NewServer(h)
	c := cmsx.NewClient(cmsx.Config{
		BaseURL:     srv.URL,
		WorkspaceID: "ws-1",
		Token:       "tok",
		Timeout:     2 * time.Second,
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
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &receivedBody)
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
	c, srv := newClient(func(w http.ResponseWriter, _ *http.Request) {
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

func TestCreateField_SerializesType(t *testing.T) {
	t.Parallel()
	var body map[string]any
	c, srv := newClient(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/fields") {
			t.Errorf("path = %q", r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
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
