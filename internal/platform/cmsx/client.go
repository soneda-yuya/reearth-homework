// Package cmsx wraps the reearth-cms Integration REST API. U-PLT shipped the
// type skeleton and a [Prober] so composition roots can wire dependencies;
// U-CSS extends it with schema-management methods (Project / Model / Field
// ensure + create). Data-plane methods (Item CRUD) arrive in later units.
package cmsx

import (
	"context"
	"net/http"
	"time"
)

// Config describes how to reach a reearth-cms instance. Token is the
// Integration API bearer token; it is read from Secret Manager at startup.
type Config struct {
	BaseURL     string
	WorkspaceID string
	Token       string
	Timeout     time.Duration
}

// Client is the REST client. Methods are added in the units that exercise
// them so the surface grows with real needs, not speculation.
type Client struct {
	cfg  Config
	http *http.Client
}

// NewClient returns a configured Client. A zero Timeout defaults to 30s,
// which is comfortable for the schema-management calls U-CSS performs
// (single-project CRUD, not bulk item I/O).
func NewClient(cfg Config) *Client {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

// Close is a no-op for the HTTP client but kept for symmetry with other
// resources that implement Close in the composition root.
func (c *Client) Close(_ context.Context) error { return nil }

// Prober adapts Client to connectserver.Prober. The real readiness check
// will ping the CMS with a lightweight HEAD once we have a stable endpoint
// for it; for now it is a stub.
func (c *Client) Prober() *ClientProber { return &ClientProber{c: c} }

// ClientProber is a readiness-probe wrapper around Client.
type ClientProber struct{ c *Client }

// Name identifies the probe in /readyz output.
func (p *ClientProber) Name() string { return "cms" }

// Probe is a stub until a real reachability check lands.
func (p *ClientProber) Probe(_ context.Context) error { return nil }
