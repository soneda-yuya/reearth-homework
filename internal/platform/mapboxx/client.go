// Package mapboxx wraps the Mapbox Geocoding REST API. There is no official
// Go SDK for Mapbox, so this package owns the HTTP client + retry / decode
// boilerplate. Application-side rate limiting is enforced by the use case
// using internal/platform/ratelimit; this client does not double-throttle.
package mapboxx

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/soneda-yuya/reearth-homework/internal/platform/retry"
	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

// Config holds Mapbox credentials and timeouts.
type Config struct {
	BaseURL     string // defaults to "https://api.mapbox.com"
	AccessToken string
	Timeout     time.Duration
}

// Client is the Mapbox wrapper.
type Client struct {
	cfg  Config
	http *http.Client
}

// NewClient returns a configured Client. Timeout 0 → 10s.
func NewClient(cfg Config) *Client {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.mapbox.com"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

// Close is a no-op placeholder kept for symmetry with other clients that
// may grow real teardown later.
func (c *Client) Close(_ context.Context) error { return nil }

// GeocodeResult is the trimmed shape we care about from Mapbox: the first
// feature's coordinate plus its relevance score (a 0..1 confidence).
type GeocodeResult struct {
	Lat       float64
	Lng       float64
	Relevance float64
	PlaceName string
}

// Geocode resolves a free-form location string. country (ISO alpha-2)
// scopes the search and dramatically improves precision for the same
// place name in different countries (e.g. "Cambridge").
//
// Returns (zero, nil) when Mapbox returns no features — callers should
// treat that as "nothing matched" and fall back to a centroid.
func (c *Client) Geocode(ctx context.Context, location, countryCdISO string) (GeocodeResult, error) {
	if location == "" {
		return GeocodeResult{}, nil
	}
	endpoint := fmt.Sprintf("%s/geocoding/v5/mapbox.places/%s.json",
		c.cfg.BaseURL, url.PathEscape(location))
	q := url.Values{}
	q.Set("access_token", c.cfg.AccessToken)
	q.Set("limit", "1")
	if countryCdISO != "" {
		q.Set("country", countryCdISO)
	}
	full := endpoint + "?" + q.Encode()

	var out GeocodeResult
	err := retry.Do(ctx, retry.DefaultPolicy, func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
		if err != nil {
			return errs.Wrap("mapbox.new_request", errs.KindInternal, err)
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return errs.Wrap("mapbox.http", errs.KindExternal, err)
		}
		defer func() { _ = resp.Body.Close() }()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return errs.Wrap("mapbox.read_body", errs.KindExternal, err)
		}

		switch {
		case resp.StatusCode == http.StatusOK:
			var parsed mapboxResponse
			if err := json.Unmarshal(body, &parsed); err != nil {
				return errs.Wrap("mapbox.decode", errs.KindInternal, err)
			}
			if len(parsed.Features) == 0 {
				out = GeocodeResult{}
				return nil
			}
			f := parsed.Features[0]
			if len(f.Center) < 2 {
				return errs.Wrap("mapbox.feature_shape", errs.KindInternal,
					fmt.Errorf("center has %d elements, want 2", len(f.Center)))
			}
			// Mapbox returns [lng, lat] order — easy to flip if you forget.
			out = GeocodeResult{
				Lng:       f.Center[0],
				Lat:       f.Center[1],
				Relevance: f.Relevance,
				PlaceName: f.PlaceName,
			}
			return nil
		case resp.StatusCode == http.StatusUnauthorized,
			resp.StatusCode == http.StatusForbidden:
			return errs.Wrap("mapbox.auth", errs.KindUnauthorized,
				fmt.Errorf("HTTP %d", resp.StatusCode))
		case resp.StatusCode == http.StatusTooManyRequests, resp.StatusCode >= 500:
			return errs.Wrap("mapbox.transient", errs.KindExternal,
				fmt.Errorf("HTTP %d", resp.StatusCode))
		default:
			return errs.Wrap("mapbox.unexpected", errs.KindInternal,
				fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body)))
		}
	})
	return out, err
}

type mapboxResponse struct {
	Features []mapboxFeature `json:"features"`
}

type mapboxFeature struct {
	PlaceName string    `json:"place_name"`
	Center    []float64 `json:"center"` // [lng, lat]
	Relevance float64   `json:"relevance"`
}
