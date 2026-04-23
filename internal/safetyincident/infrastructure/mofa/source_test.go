package mofa_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/domain"
	"github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/infrastructure/mofa"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

func TestSource_Fetch_Incremental_ParsesAndDropsBadRows(t *testing.T) {
	t.Parallel()
	body := loadFixture(t, "sample_newarrival.xml")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/newarrivalA.xml") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	src := mofa.New(srv.URL, &http.Client{Timeout: 2 * time.Second})
	items, err := src.Fetch(context.Background(), domain.IngestionModeIncremental)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	// 4 items in fixture, one dropped for bad date → 3 expected.
	if len(items) != 3 {
		t.Fatalf("len(items) = %d, want 3", len(items))
	}
	if items[0].KeyCd != "2026-04-23-001" || items[0].CountryCd != "FR" {
		t.Errorf("item[0] = %+v", items[0])
	}
	if items[1].LeaveDate.UTC().Format(time.RFC3339) != "2026-04-23T07:15:00Z" {
		t.Errorf("item[1].LeaveDate = %v", items[1].LeaveDate)
	}
}

func TestSource_Fetch_Initial_UsesInitialPath(t *testing.T) {
	t.Parallel()
	body := loadFixture(t, "sample_newarrival.xml") // reuse, payload shape is the same

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/00A.xml") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	src := mofa.New(srv.URL, &http.Client{Timeout: 2 * time.Second})
	items, err := src.Fetch(context.Background(), domain.IngestionModeInitial)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(items) == 0 {
		t.Error("expected at least one item")
	}
}

func TestSource_Fetch_TransientThenSuccess(t *testing.T) {
	t.Parallel()
	body := loadFixture(t, "sample_newarrival.xml")

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	src := mofa.New(srv.URL, &http.Client{Timeout: 2 * time.Second})
	items, err := src.Fetch(context.Background(), domain.IngestionModeIncremental)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(items) == 0 {
		t.Error("expected items after retry")
	}
	if h := hits.Load(); h < 2 {
		t.Errorf("expected >= 2 hits (retry), got %d", h)
	}
}

func TestSource_Fetch_UnknownModeReturnsError(t *testing.T) {
	t.Parallel()
	src := mofa.New("http://localhost", http.DefaultClient)
	_, err := src.Fetch(context.Background(), domain.IngestionMode("unknown"))
	if err == nil {
		t.Fatal("expected error for unknown mode")
	}
}
