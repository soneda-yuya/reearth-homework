package geocode

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/domain"
	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
)

// CentroidLookup is the subset of CentroidGeocoder the chain depends on.
// Declared as an interface so the chain test can stub centroid lookups
// without loading the embedded JSON.
type CentroidLookup interface {
	Lookup(countryCd string) (domain.Point, error)
}

// MapboxLookup is the subset of MapboxGeocoder the chain calls.
type MapboxLookup interface {
	Lookup(ctx context.Context, location, countryCd string) (MapboxHit, bool, error)
}

// ChainGeocoder is the production Geocoder. It tries Mapbox first; on
// success it also propagates Mapbox's ISO country so the caller can backfill
// MOFA items that shipped no <country> element. On Mapbox miss or transient
// failure the chain falls back to the country centroid — which itself needs a
// country_cd, so items still missing one after Mapbox surface as
// KindInvalidInput and are dropped by the use case.
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
// the returned Point; CountryCd is filled when Mapbox was able to derive one
// and the caller-supplied countryCd was empty.
func (g *ChainGeocoder) Geocode(ctx context.Context, location, countryCd string) (domain.GeocodeResult, error) {
	if location != "" {
		hit, ok, err := g.mapbox.Lookup(ctx, location, countryCd)
		switch {
		case err != nil:
			// Treat transport / auth errors as a soft miss: the centroid is
			// still useful if we know the country. The error is recorded in
			// logs so an operator can see degraded geocoding even though the
			// run keeps moving.
			g.logger.WarnContext(ctx, "mapbox lookup failed; falling back to centroid",
				"app.ingestion.phase", "geocode",
				"country_cd", countryCd,
				"location", location,
				"err", err,
			)
		case ok:
			// Mapbox's ISO country supersedes the incoming value only when
			// the incoming one is empty; otherwise MOFA's code (which we
			// already normalised to ISO upstream) wins.
			resolvedCountry := countryCd
			if resolvedCountry == "" {
				resolvedCountry = hit.CountryCd
			}
			return domain.GeocodeResult{
				Point:     hit.Point,
				Source:    domain.GeocodeSourceMapbox,
				CountryCd: resolvedCountry,
			}, nil
		}
	}

	if countryCd == "" {
		// Mapbox could not place the item *and* MOFA gave us no country —
		// centroid would have nothing to look up. Classify as invalid input
		// so the use case drops the item with a clear reason instead of
		// blowing up the centroid lookup with "country_cd is required".
		return domain.GeocodeResult{}, errs.Wrap("geocode.chain", errs.KindInvalidInput,
			fmt.Errorf("cannot geocode item: mapbox miss and no country_cd"))
	}

	point, err := g.centroid.Lookup(countryCd)
	if err != nil {
		return domain.GeocodeResult{}, err
	}
	return domain.GeocodeResult{
		Point:     point,
		Source:    domain.GeocodeSourceCountryCentroid,
		CountryCd: countryCd,
	}, nil
}
