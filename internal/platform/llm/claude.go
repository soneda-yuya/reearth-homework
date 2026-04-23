// Package llm wraps the LLM HTTP APIs we call from application code. The
// only concrete implementation today is Anthropic Claude (used by U-ING for
// location extraction); the package name stays generic so a future Gemini /
// GPT client can land alongside without refactoring callers.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/soneda-yuya/reearth-homework/internal/platform/retry"
	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

// Config configures the Claude client. Model is the Anthropic model id (e.g.
// "claude-haiku-4-5"). Token caps the per-message generation length —
// extraction prompts only need a couple of dozen tokens.
type Config struct {
	BaseURL    string
	APIKey     string
	Model      string
	MaxTokens  int
	Timeout    time.Duration
	APIVersion string // "anthropic-version" header; defaults to "2023-06-01"
}

// Client talks to the Anthropic Messages API. It is concurrency-safe; the
// only mutable state is the retained http.Client.
type Client struct {
	cfg  Config
	http *http.Client
}

// NewClient returns a configured Client. Timeout 0 → 30s; MaxTokens 0 → 256.
func NewClient(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.anthropic.com"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 256
	}
	if cfg.APIVersion == "" {
		cfg.APIVersion = "2023-06-01"
	}
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

// Complete sends a system + user prompt and returns the assistant's text
// content. Multi-turn / tool use are deliberately out of scope — extractor
// callers only need a single-shot string back.
func (c *Client) Complete(ctx context.Context, system, user string) (string, error) {
	body, err := json.Marshal(messagesRequest{
		Model:     c.cfg.Model,
		MaxTokens: c.cfg.MaxTokens,
		System:    system,
		Messages:  []message{{Role: "user", Content: user}},
	})
	if err != nil {
		return "", errs.Wrap("llm.marshal", errs.KindInternal, err)
	}

	var out string
	err = retry.Do(ctx, retry.DefaultPolicy, func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.cfg.BaseURL+"/v1/messages", bytes.NewReader(body))
		if err != nil {
			return errs.Wrap("llm.new_request", errs.KindInternal, err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-api-key", c.cfg.APIKey)
		req.Header.Set("anthropic-version", c.cfg.APIVersion)

		resp, err := c.http.Do(req)
		if err != nil {
			return errs.Wrap("llm.http", errs.KindExternal, err)
		}
		defer func() { _ = resp.Body.Close() }()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return errs.Wrap("llm.read_body", errs.KindExternal, err)
		}

		switch {
		case resp.StatusCode == http.StatusOK:
			var parsed messagesResponse
			if err := json.Unmarshal(respBody, &parsed); err != nil {
				return errs.Wrap("llm.decode", errs.KindInternal, err)
			}
			out = firstText(parsed)
			return nil
		case resp.StatusCode == http.StatusUnauthorized,
			resp.StatusCode == http.StatusForbidden:
			return errs.Wrap("llm.auth", errs.KindUnauthorized,
				fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody)))
		case resp.StatusCode == http.StatusTooManyRequests, resp.StatusCode >= 500:
			return errs.Wrap("llm.transient", errs.KindExternal,
				fmt.Errorf("HTTP %d", resp.StatusCode))
		default:
			return errs.Wrap("llm.unexpected", errs.KindInternal,
				fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody)))
		}
	})
	return out, err
}

// firstText extracts the first "text" content block from a Messages
// response. Anthropic models can mix tool_use / text in a single response
// but for our single-shot prompts the first text block is the answer.
func firstText(r messagesResponse) string {
	for _, c := range r.Content {
		if c.Type == "text" {
			return c.Text
		}
	}
	return ""
}

// --- wire types -----------------------------------------------------------

type messagesRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system,omitempty"`
	Messages  []message `json:"messages"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type messagesResponse struct {
	ID      string         `json:"id"`
	Model   string         `json:"model"`
	Content []contentBlock `json:"content"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}
