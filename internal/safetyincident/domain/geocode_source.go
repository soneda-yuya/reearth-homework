package domain

import "fmt"

// GeocodeSource identifies which stage of the Geocoder chain produced the
// final coordinate. It surfaces in CMS, in metrics, and in Flutter UI ("国
// レベル" badge) so the wire spelling is part of the API contract.
type GeocodeSource int

const (
	GeocodeSourceUnspecified GeocodeSource = iota
	GeocodeSourceMapbox
	GeocodeSourceCountryCentroid
)

// String returns the canonical wire name. The mapping mirrors proto
// `GeocodeSource` values without the GEOCODE_SOURCE_ prefix.
func (s GeocodeSource) String() string {
	switch s {
	case GeocodeSourceMapbox:
		return "mapbox"
	case GeocodeSourceCountryCentroid:
		return "country_centroid"
	default:
		return fmt.Sprintf("unspecified(%d)", int(s))
	}
}

// IsValid reports whether the value is one of the declared constants.
func (s GeocodeSource) IsValid() bool {
	switch s {
	case GeocodeSourceMapbox, GeocodeSourceCountryCentroid:
		return true
	default:
		return false
	}
}
