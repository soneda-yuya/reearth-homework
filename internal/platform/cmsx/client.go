// Package cmsx wraps the reearth-cms Integration REST API. The concrete HTTP
// client is implemented in later units (U-CSS / U-ING / U-BFF) as they each
// exercise a different subset. U-PLT ships only the type skeleton and the
// [Prober] shape so service composition roots can wire dependencies.
package cmsx

import (
	"context"
	"time"
)

// Config describes how to reach a reearth-cms instance.
type Config struct {
	BaseURL     string
	WorkspaceID string
	Token       string // resolved secret value
	Timeout     time.Duration
}

// Client is a placeholder HTTP client. Methods will be added by the units that
// actually call the integration API.
type Client struct {
	cfg Config
}

// NewClient returns an unconfigured Client. Real network setup happens inside
// the methods added by downstream units.
func NewClient(cfg Config) *Client {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	return &Client{cfg: cfg}
}

// Close is a no-op for now but keeps the Closer contract used by the
// composition roots.
func (c *Client) Close(ctx context.Context) error { return nil }

// Prober adapts Client to connectserver.Prober (added when the real client ships).
func (c *Client) Prober() *ClientProber { return &ClientProber{c: c} }

// ClientProber is a readiness probe wrapper.
type ClientProber struct{ c *Client }

// Name identifies the probe in /readyz output.
func (p *ClientProber) Name() string { return "cms" }

// Probe is a stub until a real reachability check lands.
func (p *ClientProber) Probe(ctx context.Context) error { return nil }
