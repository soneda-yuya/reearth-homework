// Package mapboxx wraps Mapbox Geocoding. No official Go SDK exists, so the
// real HTTP client is implemented in U-ING. U-PLT ships the shape and the
// rate-limit / retry wiring.
package mapboxx

import (
	"context"
	"time"

	"github.com/soneda-yuya/reearth-homework/internal/platform/ratelimit"
	"github.com/soneda-yuya/reearth-homework/internal/platform/retry"
)

// Config holds Mapbox credentials and tuning.
type Config struct {
	AccessToken string
	Timeout     time.Duration
	RPM         int
	Burst       int
}

// Client is the Mapbox wrapper. Real HTTP calls are added in U-ING.
type Client struct {
	cfg     Config
	limiter *ratelimit.Limiter
	policy  retry.Policy
}

// NewClient returns a preconfigured Client.
func NewClient(cfg Config) *Client {
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.RPM == 0 {
		cfg.RPM = 600
	}
	if cfg.Burst == 0 {
		cfg.Burst = 10
	}
	return &Client{
		cfg:     cfg,
		limiter: ratelimit.New(cfg.RPM, cfg.Burst, "mapbox"),
		policy:  retry.DefaultPolicy,
	}
}

// Close is a no-op placeholder.
func (c *Client) Close(ctx context.Context) error { return nil }
