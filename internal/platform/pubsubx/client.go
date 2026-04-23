// Package pubsubx wraps Google Cloud Pub/Sub client construction. The thin
// wrapper exists so the rest of the codebase doesn't import the SDK
// directly — easier to swap implementations and to keep test seams clean.
package pubsubx

import (
	"context"
	"fmt"

	pubsub "cloud.google.com/go/pubsub/v2"

	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

// Config identifies the Pub/Sub project. Endpoint is optional — set it to
// the emulator's address (e.g. "localhost:8085") when running tests against
// the official Pub/Sub emulator.
type Config struct {
	ProjectID string
	Endpoint  string
}

// Client wraps the cloud.google.com/go/pubsub Client. Callers receive a
// Topic via Topic(); the Client.Close should be deferred at process exit.
type Client struct {
	cfg   Config
	inner *pubsub.Client
}

// NewClient instantiates the underlying SDK client. Authentication is
// handled by Application Default Credentials, so production picks up the
// Cloud Run Job runtime SA automatically.
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.ProjectID == "" {
		return nil, errs.Wrap("pubsubx.config", errs.KindInvalidInput,
			fmt.Errorf("ProjectID is required"))
	}
	c, err := pubsub.NewClient(ctx, cfg.ProjectID)
	if err != nil {
		return nil, errs.Wrap("pubsubx.new_client", errs.KindExternal, err)
	}
	return &Client{cfg: cfg, inner: c}, nil
}

// ProjectID returns the configured project id.
func (c *Client) ProjectID() string { return c.cfg.ProjectID }

// Close releases the underlying gRPC connections.
func (c *Client) Close(_ context.Context) error {
	if c.inner == nil {
		return nil
	}
	return c.inner.Close()
}

// Topic returns a Publisher-handle bound to the named topic. Pass either a
// short topic id ("my-topic") or a fully-qualified name
// ("projects/p/topics/my-topic"). The topic must already exist (created by
// Terraform); this client does not auto-create topics.
func (c *Client) Topic(name string) *Topic {
	return &Topic{inner: c.inner.Publisher(name)}
}

// Topic is the Publish-side handle adapter code calls. The interface
// satisfies internal/safetyincident/infrastructure/eventbus.Topic.
type Topic struct {
	inner *pubsub.Publisher
}

// Publish enqueues data + attributes onto the topic. The returned id is the
// server-assigned message id (or empty when the broker did not echo one).
// Stop() is called by Client.Close at process exit, so callers do not need
// to flush manually.
func (t *Topic) Publish(ctx context.Context, data []byte, attributes map[string]string) (string, error) {
	res := t.inner.Publish(ctx, &pubsub.Message{Data: data, Attributes: attributes})
	id, err := res.Get(ctx)
	if err != nil {
		return "", errs.Wrap("pubsubx.publish", errs.KindExternal, err)
	}
	return id, nil
}

// Stop drains the topic's publisher buffer. Safe to call multiple times.
func (t *Topic) Stop() {
	if t.inner != nil {
		t.inner.Stop()
	}
}
