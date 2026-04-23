// Package geocode is the safetyincident geocoding adapter. It owns the
// chain that resolves an LLM-extracted location into a coordinate, with a
// country centroid as the safety net.
package geocode

import (
	"context"

	"github.com/soneda-yuya/reearth-homework/internal/platform/mapboxx"
	"github.com/soneda-yuya/reearth-homework/internal/safetyincident/domain"
)

// MapboxClient is the subset of mapboxx.Client we use. Declared as an
// interface so the chain test can plug in a stub without an HTTP server.
type MapboxClient interface {
	Geocode(ctx context.Context, location, countryCdISO string) (mapboxx.GeocodeResult, error)
}

// MapboxGeocoder is the first link in the chain. It returns a Point if
// Mapbox is confident enough; the caller decides what to do with low
// relevance / empty result (see ChainGeocoder).
type MapboxGeocoder struct {
	client       MapboxClient
	minRelevance float64
}

// NewMapboxGeocoder configures a MapboxGeocoder. minRelevance ∈ [0, 1] —
// 0.5 is the default chain threshold and matches U-ING design §1.4.3.
func NewMapboxGeocoder(client MapboxClient, minRelevance float64) *MapboxGeocoder {
	if minRelevance < 0 {
		minRelevance = 0
	}
	return &MapboxGeocoder{client: client, minRelevance: minRelevance}
}

// Lookup returns ok=false when Mapbox returned no usable feature. Errors
// flow up — the chain decides whether to swallow them and fall through to
// the centroid.
func (g *MapboxGeocoder) Lookup(ctx context.Context, location, countryCd string) (domain.Point, bool, error) {
	if location == "" {
		return domain.Point{}, false, nil
	}
	result, err := g.client.Geocode(ctx, location, countryCd)
	if err != nil {
		return domain.Point{}, false, err
	}
	if result == (mapboxx.GeocodeResult{}) || result.Relevance < g.minRelevance {
		return domain.Point{}, false, nil
	}
	return domain.Point{Lat: result.Lat, Lng: result.Lng}, true, nil
}
