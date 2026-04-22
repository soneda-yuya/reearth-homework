package connectserver

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// Prober reports readiness for a single dependency. Probe must return within
// the timeout set on the ReadyzHandler; implementations should honour ctx.
type Prober interface {
	Name() string
	Probe(ctx context.Context) error
}

// HealthzHandler always returns 200. Cloud Run uses it for liveness checks.
func HealthzHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	}
}

// ReadyzHandler runs every prober concurrently. It returns 200 only if all
// probers succeed within timeout.
func ReadyzHandler(probers []Prober, timeout time.Duration) http.HandlerFunc {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()

		type result struct {
			Name string `json:"name"`
			OK   bool   `json:"ok"`
			Err  string `json:"err,omitempty"`
		}
		results := make([]result, len(probers))
		done := make(chan struct{}, len(probers))
		for i, p := range probers {
			i, p := i, p
			go func() {
				err := p.Probe(ctx)
				results[i] = result{Name: p.Name(), OK: err == nil}
				if err != nil {
					results[i].Err = err.Error()
				}
				done <- struct{}{}
			}()
		}
		for range probers {
			<-done
		}

		allOK := true
		for _, r := range results {
			if !r.OK {
				allOK = false
				break
			}
		}
		status := http.StatusOK
		if !allOK {
			status = http.StatusServiceUnavailable
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  statusLabel(allOK),
			"probers": results,
		})
	}
}

func statusLabel(ok bool) string {
	if ok {
		return "ready"
	}
	return "not_ready"
}
