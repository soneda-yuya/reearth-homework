// Package application holds the choropleth / heatmap aggregators that turn
// raw safety incidents into map-ready views. The aggregator is an in-memory
// operation today; if the corpus grows large enough to warrant pushing the
// work into reearth-cms or BigQuery, this interface is the only thing that
// needs to change.
package application

import (
	"context"

	crimemap "github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/crimemap/domain"
	safetyincident "github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/domain"
)

// aggregationCap is the upper bound passed to the reader when we know the
// caller does not want cursor paging. The choropleth / heatmap views need a
// whole snapshot of the current time-window to normalise colours and weights.
const aggregationCap = 10000

// Aggregator computes choropleth and heatmap results over a time window. It
// owns only the reader dependency; colour mapping and centroid exclusion are
// pure domain logic in internal/safetyincident/crimemap/domain.
type Aggregator struct {
	reader safetyincident.SafetyIncidentReader
}

// NewAggregator wires the reader dependency.
func NewAggregator(reader safetyincident.SafetyIncidentReader) *Aggregator {
	return &Aggregator{reader: reader}
}

// Choropleth counts incidents per country inside the time window, then paints
// each country using ColorFromCount against the country with the highest
// count. An empty corpus returns an empty-but-non-nil Items slice so the
// client can render an "all quiet" legend without a nil check.
func (a *Aggregator) Choropleth(ctx context.Context, filter crimemap.CrimeMapFilter) (crimemap.ChoroplethResult, error) {
	items, _, err := a.reader.List(ctx, safetyincident.ListFilter{
		LeaveFrom: filter.LeaveFrom,
		LeaveTo:   filter.LeaveTo,
		Limit:     aggregationCap,
	})
	if err != nil {
		return crimemap.ChoroplethResult{}, err
	}

	counts := make(map[string]crimemap.CountryChoropleth, len(items))
	for _, it := range items {
		entry := counts[it.CountryCd]
		entry.CountryCd = it.CountryCd
		entry.CountryName = it.CountryName
		entry.Count++
		counts[it.CountryCd] = entry
	}

	maxCount := 0
	for _, v := range counts {
		if v.Count > maxCount {
			maxCount = v.Count
		}
	}

	out := make([]crimemap.CountryChoropleth, 0, len(counts))
	for _, v := range counts {
		v.Color = crimemap.ColorFromCount(v.Count, maxCount)
		out = append(out, v)
	}
	return crimemap.ChoroplethResult{Items: out, Total: len(items)}, nil
}

// Heatmap returns the raw point cloud, excluding incidents whose geocoded
// coordinate came from the country-centroid fallback (those would cluster an
// entire nation onto one pin and drown real sightings). The excluded count
// lets the UI surface data-quality warnings when fallbacks dominate.
func (a *Aggregator) Heatmap(ctx context.Context, filter crimemap.CrimeMapFilter) (crimemap.HeatmapResult, error) {
	items, _, err := a.reader.List(ctx, safetyincident.ListFilter{
		LeaveFrom: filter.LeaveFrom,
		LeaveTo:   filter.LeaveTo,
		Limit:     aggregationCap,
	})
	if err != nil {
		return crimemap.HeatmapResult{}, err
	}

	points := make([]crimemap.HeatmapPoint, 0, len(items))
	excluded := 0
	for _, it := range items {
		if it.GeocodeSource == safetyincident.GeocodeSourceCountryCentroid {
			excluded++
			continue
		}
		points = append(points, crimemap.HeatmapPoint{
			Location: it.Geometry,
			Weight:   1.0,
		})
	}
	return crimemap.HeatmapResult{Points: points, ExcludedFallback: excluded}, nil
}
