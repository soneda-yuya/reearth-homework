package connectserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/soneda-yuya/overseas-safety-map/internal/platform/connectserver"
)

type fakeProber struct {
	name string
	err  error
}

func (f fakeProber) Name() string                    { return f.name }
func (f fakeProber) Probe(ctx context.Context) error { return f.err }

func TestHealthz_AlwaysOK(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	connectserver.HealthzHandler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("Healthz status = %d, want 200", rr.Code)
	}
}

func TestReadyz_AllOK(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	handler := connectserver.ReadyzHandler([]connectserver.Prober{
		fakeProber{name: "cms"},
		fakeProber{name: "firebase"},
	}, time.Second)
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("Readyz status = %d, want 200", rr.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(readAll(t, rr.Body), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ready" {
		t.Errorf("status = %v, want ready", body["status"])
	}
}

func TestReadyz_OneFails(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	handler := connectserver.ReadyzHandler([]connectserver.Prober{
		fakeProber{name: "cms"},
		fakeProber{name: "firebase", err: errors.New("down")},
	}, time.Second)
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("Readyz status = %d, want 503", rr.Code)
	}
}

func readAll(t *testing.T, r interface{ Read([]byte) (int, error) }) []byte {
	t.Helper()
	b, err := io.ReadAll(anyReader{r})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type anyReader struct {
	r interface{ Read([]byte) (int, error) }
}

func (a anyReader) Read(p []byte) (int, error) { return a.r.Read(p) }
