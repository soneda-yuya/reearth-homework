// Package domain holds the crimemap subdomain of the safetyincident bounded
// context: aggregated views over raw safety incidents that the BFF exposes
// as choropleth and heatmap RPCs.
package domain

import (
	"time"

	safetyincident "github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/domain"
)

// CrimeMapFilter scopes a Choropleth or Heatmap aggregation by publish-time
// window. Zero values mean the caller did not constrain that side of the
// range; the reader treats them as open-ended.
type CrimeMapFilter struct {
	LeaveFrom time.Time
	LeaveTo   time.Time
}

// CountryChoropleth is one cell of a choropleth map: a country plus the
// number of incidents in the filter window and the colour the server chose
// for it (so every client paints the same shade of red).
type CountryChoropleth struct {
	CountryCd   string
	CountryName string
	Count       int
	Color       string // "#rrggbb"
}

// ChoroplethResult is the whole map plus a Total so clients can render a
// legend without recomputing.
type ChoroplethResult struct {
	Items []CountryChoropleth
	Total int
}

// HeatmapPoint is a single incident location. Weight exists for future
// severity-based weighting; MVP clients all pass 1.0.
type HeatmapPoint struct {
	Location safetyincident.Point
	Weight   float64
}

// HeatmapResult carries the raw point cloud plus a count of incidents whose
// geocoded location was only the country centroid — those are dropped from
// the heatmap because they would otherwise cluster a whole nation onto one
// pin and swamp the real coordinates.
type HeatmapResult struct {
	Points           []HeatmapPoint
	ExcludedFallback int
}
