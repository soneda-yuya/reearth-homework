// Package pubsubx wraps Google Cloud Pub/Sub client construction. The actual
// pubsub SDK dependency is added in U-ING (publisher) and U-NTF (subscriber);
// U-PLT ships only the factory shape.
package pubsubx

import "context"

// Config identifies the Pub/Sub project.
type Config struct {
	ProjectID string
}

// Client is a placeholder until the real cloud.google.com/go/pubsub client is wired.
type Client struct {
	cfg Config
}

// NewClient returns an unconfigured Client.
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	return &Client{cfg: cfg}, nil
}

// ProjectID returns the project id.
func (c *Client) ProjectID() string { return c.cfg.ProjectID }

// Close is a no-op placeholder.
func (c *Client) Close(ctx context.Context) error { return nil }
