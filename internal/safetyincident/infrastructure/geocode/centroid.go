package geocode

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/domain"
	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
)

// centroidsRaw is the embedded JSON shipped at build time. The data is
// public domain (Natural Earth). See country_centroids.json for the
// attribution and caveats.
//
//go:embed country_centroids.json
var centroidsRaw []byte

// CentroidGeocoder is the second link in the chain — never fails for known
// countries, always returns the country's geographic centroid.
type CentroidGeocoder struct {
	table map[string]domain.Point
}

// NewCentroidGeocoder loads the embedded country_centroids.json. The leading
// "_source" entry is filtered out because it is metadata, not a country.
func NewCentroidGeocoder() (*CentroidGeocoder, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(centroidsRaw, &raw); err != nil {
		return nil, errs.Wrap("centroid.load", errs.KindInternal, err)
	}
	table := make(map[string]domain.Point, len(raw))
	for k, v := range raw {
		if strings.HasPrefix(k, "_") {
			continue
		}
		var p domain.Point
		if err := json.Unmarshal(v, &p); err != nil {
			return nil, errs.Wrap("centroid.entry",
				errs.KindInternal,
				fmt.Errorf("country %q: %w", k, err))
		}
		// Validate eagerly so a bad data ship-out fails at startup, not at
		// the first lookup.
		if err := p.Validate(); err != nil {
			return nil, errs.Wrap("centroid.entry",
				errs.KindInternal,
				fmt.Errorf("country %q: %w", k, err))
		}
		table[strings.ToUpper(k)] = p
	}
	return &CentroidGeocoder{table: table}, nil
}

// Lookup returns the centroid for the given ISO alpha-2 country. Unknown
// codes produce a KindNotFound error so the chain caller can record a
// per-item failure instead of silently shrugging.
func (g *CentroidGeocoder) Lookup(countryCd string) (domain.Point, error) {
	if countryCd == "" {
		return domain.Point{}, errs.Wrap("centroid.lookup",
			errs.KindInvalidInput,
			fmt.Errorf("country_cd is required"))
	}
	p, ok := g.table[strings.ToUpper(countryCd)]
	if !ok {
		return domain.Point{}, errs.Wrap("centroid.lookup",
			errs.KindNotFound,
			fmt.Errorf("no centroid for country %q", countryCd))
	}
	return p, nil
}

// Size returns the number of countries currently loaded — used for
// observability / smoke tests.
func (g *CentroidGeocoder) Size() int { return len(g.table) }
