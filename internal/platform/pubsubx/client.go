// Package pubsubx wraps Google Cloud Pub/Sub client construction. The thin
// wrapper exists so the rest of the codebase doesn't import the SDK
// directly — easier to swap implementations and to keep test seams clean.
package pubsubx

import (
	"context"
	"fmt"
	"sync"

	pubsub "cloud.google.com/go/pubsub/v2"

	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

// Config identifies the Pub/Sub project. Authentication is handled by
// Application Default Credentials. Local emulator support (PUBSUB_EMULATOR_HOST
// env) is provided by the SDK itself; this package does not need extra
// configuration to use it.
type Config struct {
	ProjectID string
}

// Client wraps the cloud.google.com/go/pubsub Client. Callers receive a
// Topic via Topic(); Close drains every Topic this Client created and then
// closes the underlying gRPC connections.
type Client struct {
	cfg   Config
	inner *pubsub.Client

	mu     sync.Mutex
	topics []*Topic
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

// Close drains every Topic created from this Client (so buffered publishes
// are flushed) and then releases the underlying gRPC connections. Safe to
// call once at process exit; callers do not need to defer Topic.Stop()
// individually unless they want eager flush before Close.
func (c *Client) Close(_ context.Context) error {
	c.mu.Lock()
	topics := c.topics
	c.topics = nil
	c.mu.Unlock()
	for _, t := range topics {
		t.Stop()
	}
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
	t := &Topic{inner: c.inner.Publisher(name)}
	c.mu.Lock()
	c.topics = append(c.topics, t)
	c.mu.Unlock()
	return t
}

// Topic is the Publish-side handle adapter code calls. The interface
// satisfies internal/safetyincident/infrastructure/eventbus.Topic.
type Topic struct {
	inner *pubsub.Publisher
}

// Publish enqueues data + attributes onto the topic. The returned id is the
// server-assigned message id (or empty when the broker did not echo one).
// Buffered messages are flushed when Stop() is called; Client.Close stops
// every Topic the client created, so callers do not need to flush manually.
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
