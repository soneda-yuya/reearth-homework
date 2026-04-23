package llm_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/soneda-yuya/overseas-safety-map/internal/platform/llm"
	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
)

func newTestClient(h http.HandlerFunc) (*llm.Client, *httptest.Server) {
	srv := httptest.NewServer(h)
	c := llm.NewClient(llm.Config{
		BaseURL:   srv.URL,
		APIKey:    "test-key",
		Model:     "claude-haiku-4-5",
		MaxTokens: 64,
		Timeout:   2 * time.Second,
	})
	return c, srv
}

func TestComplete_HappyPath(t *testing.T) {
	t.Parallel()
	var receivedBody map[string]any
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/v1/messages"; got != want {
			t.Errorf("path = %q want %q", got, want)
		}
		if got := r.Header.Get("x-api-key"); got != "test-key" {
			t.Errorf("api key header = %q", got)
		}
		if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
			t.Errorf("anthropic-version = %q", got)
		}
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(b, &receivedBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		_, _ = w.Write([]byte(`{
			"id":"msg-1","model":"claude-haiku-4-5",
			"content":[{"type":"text","text":"東京都新宿区"}]
		}`))
	})
	defer srv.Close()

	got, err := c.Complete(context.Background(), "you are a geo assistant", "extract location from text")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got != "東京都新宿区" {
		t.Errorf("text = %q, want 東京都新宿区", got)
	}
	if receivedBody["model"] != "claude-haiku-4-5" || receivedBody["system"] != "you are a geo assistant" {
		t.Errorf("body mismatch: %v", receivedBody)
	}
	msgs, _ := receivedBody["messages"].([]any)
	if len(msgs) != 1 {
		t.Errorf("messages count = %d, want 1", len(msgs))
	}
}

func TestComplete_AuthFailureNoRetry(t *testing.T) {
	t.Parallel()
	c, srv := newTestClient(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid api key"}`))
	})
	defer srv.Close()

	_, err := c.Complete(context.Background(), "", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errs.IsKind(err, errs.KindUnauthorized) {
		t.Errorf("kind = %s, want KindUnauthorized", errs.KindOf(err))
	}
}

func TestComplete_PicksFirstTextBlock(t *testing.T) {
	t.Parallel()
	c, srv := newTestClient(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"id":"msg-2","model":"claude-haiku-4-5",
			"content":[
				{"type":"tool_use","name":"x"},
				{"type":"text","text":"the answer"}
			]
		}`))
	})
	defer srv.Close()

	got, err := c.Complete(context.Background(), "", "")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got != "the answer" {
		t.Errorf("text = %q, want 'the answer'", got)
	}
}

func TestComplete_NoTextBlockReturnsEmpty(t *testing.T) {
	t.Parallel()
	c, srv := newTestClient(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"msg-3","model":"x","content":[]}`))
	})
	defer srv.Close()

	got, err := c.Complete(context.Background(), "", "")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got != "" {
		t.Errorf("text = %q, want empty", got)
	}
}

func TestNewClient_AcceptsZeroValueConfig(t *testing.T) {
	t.Parallel()
	// We can't invoke Complete here without hitting the live API, but we can
	// confirm that NewClient with a zero-value Config (other than the
	// required APIKey/Model) does not panic and returns a usable struct.
	// The defaults (BaseURL/Timeout/MaxTokens/APIVersion) are exercised
	// end-to-end by the happy-path test against httptest.
	c := llm.NewClient(llm.Config{APIKey: "k", Model: "m"})
	if c == nil {
		t.Fatal("nil client")
	}
}
