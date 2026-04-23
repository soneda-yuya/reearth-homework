package geocode

import (
	"context"
	"log/slog"

	"github.com/soneda-yuya/reearth-homework/internal/safetyincident/domain"
)

// CentroidLookup is the subset of CentroidGeocoder the chain depends on.
// Declared as an interface so the chain test can stub centroid lookups
// without loading the embedded JSON.
type CentroidLookup interface {
	Lookup(countryCd string) (domain.Point, error)
}

// MapboxLookup is the subset of MapboxGeocoder the chain calls.
type MapboxLookup interface {
	Lookup(ctx context.Context, location, countryCd string) (domain.Point, bool, error)
}

// ChainGeocoder is the production Geocoder. It tries Mapbox first; on miss
// or transient failure it falls back to the country centroid. The Mapbox
// failure is logged but never propagated — the centroid is "good enough"
// for the user, and Failed[geocode] is reserved for "we can't even resolve
// the country".
type ChainGeocoder struct {
	mapbox   MapboxLookup
	centroid CentroidLookup
	logger   *slog.Logger
}

// NewChainGeocoder wires the chain. A nil logger falls back to slog.Default.
func NewChainGeocoder(mapbox MapboxLookup, centroid CentroidLookup, logger *slog.Logger) *ChainGeocoder {
	if logger == nil {
		logger = slog.Default()
	}
	return &ChainGeocoder{mapbox: mapbox, centroid: centroid, logger: logger}
}

// Geocode implements domain.Geocoder. Source identifies which stage produced
// the returned Point.
func (g *ChainGeocoder) Geocode(ctx context.Context, location, countryCd string) (domain.GeocodeResult, error) {
	if location != "" {
		point, ok, err := g.mapbox.Lookup(ctx, location, countryCd)
		switch {
		case err != nil:
			// Treat transport / auth errors as a soft miss: the centroid is
			// still useful. We record the error in logs so an operator can
			// see degraded geocoding even though the run keeps moving.
			g.logger.WarnContext(ctx, "mapbox lookup failed; falling back to centroid",
				"app.ingestion.phase", "geocode",
				"country_cd", countryCd,
				"location", location,
				"err", err,
			)
		case ok:
			return domain.GeocodeResult{Point: point, Source: domain.GeocodeSourceMapbox}, nil
		}
	}

	point, err := g.centroid.Lookup(countryCd)
	if err != nil {
		return domain.GeocodeResult{}, err
	}
	return domain.GeocodeResult{Point: point, Source: domain.GeocodeSourceCountryCentroid}, nil
}
