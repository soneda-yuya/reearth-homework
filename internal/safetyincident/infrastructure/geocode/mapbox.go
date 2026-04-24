// Package geocode is the safetyincident geocoding adapter. It owns the
// chain that resolves an LLM-extracted location into a coordinate, with a
// country centroid as the safety net.
package geocode

import (
	"context"

	"github.com/soneda-yuya/overseas-safety-map/internal/platform/mapboxx"
	"github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/domain"
)

// MapboxClient is the subset of mapboxx.Client we use. Declared as an
// interface so the chain test can plug in a stub without an HTTP server.
type MapboxClient interface {
	Geocode(ctx context.Context, location, countryCdISO string) (mapboxx.GeocodeResult, error)
}

// MapboxGeocoder is the first link in the chain. It returns a MapboxHit
// (Point + ISO country derived from Mapbox's context hierarchy) when
// Mapbox is confident enough; the caller decides what to do with low
// relevance / empty result (see ChainGeocoder).
type MapboxGeocoder struct {
	client       MapboxClient
	minRelevance float64
}

// MapboxHit is the chain-local result shape. Kept separate from
// domain.GeocodeResult so this package does not leak its internal "bool ok"
// signal into the domain port.
type MapboxHit struct {
	Point     domain.Point
	CountryCd string
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
// the centroid. CountryCd on the hit is populated from Mapbox's context
// (ISO alpha-2) when the feature is inside a country; otherwise "".
func (g *MapboxGeocoder) Lookup(ctx context.Context, location, countryCd string) (MapboxHit, bool, error) {
	if location == "" {
		return MapboxHit{}, false, nil
	}
	result, err := g.client.Geocode(ctx, location, countryCd)
	if err != nil {
		return MapboxHit{}, false, err
	}
	if result == (mapboxx.GeocodeResult{}) || result.Relevance < g.minRelevance {
		return MapboxHit{}, false, nil
	}
	return MapboxHit{
		Point:     domain.Point{Lat: result.Lat, Lng: result.Lng},
		CountryCd: result.CountryCd,
	}, true, nil
}
