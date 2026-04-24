package domain

import "time"

// SafetyIncident is the post-processing form of a MOFA item: original metadata
// plus the LLM-extracted location and the resolved coordinate. This is the
// shape we persist in reearth-cms.
type SafetyIncident struct {
	MailItem
	ExtractedLocation string
	Geometry          Point
	GeocodeSource     GeocodeSource
	IngestedAt        time.Time
	UpdatedAt         time.Time
}

// Build composes a SafetyIncident from its three sources of truth: the raw
// MailItem, the LLM extraction, and the geocoding result. now is injected so
// tests can pin IngestedAt/UpdatedAt without a real clock.
//
// The fallback semantics live in the Geocoder chain — by the time we reach
// here, geocode.Source already tells us whether the coordinate came from
// Mapbox or the country centroid. When the MailItem lacks a country_cd
// (MOFA occasionally ships items without <country>) we backfill from the
// geocoder result, which extracted one from Mapbox's context.
func Build(item MailItem, extract ExtractResult, geocode GeocodeResult, now time.Time) SafetyIncident {
	if item.CountryCd == "" {
		item.CountryCd = geocode.CountryCd
	}
	return SafetyIncident{
		MailItem:          item,
		ExtractedLocation: extract.Location,
		Geometry:          geocode.Point,
		GeocodeSource:     geocode.Source,
		IngestedAt:        now,
		UpdatedAt:         now,
	}
}
