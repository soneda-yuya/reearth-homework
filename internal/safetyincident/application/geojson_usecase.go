package application

import (
	"context"

	"github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/domain"
)

// GeoJSONUseCase returns an RFC 7946 FeatureCollection covering one page of
// incidents. The MVP reads a single page with a wide Limit so the whole map
// fits in one response; a future iteration may introduce streaming.
type GeoJSONUseCase struct {
	reader domain.SafetyIncidentReader
}

// NewGeoJSONUseCase wires the reader dependency.
func NewGeoJSONUseCase(reader domain.SafetyIncidentReader) *GeoJSONUseCase {
	return &GeoJSONUseCase{reader: reader}
}

// FeatureCollection is the decoded GeoJSON shape. The RPC handler marshals
// this to the wire, either as JSON bytes inside a Struct field or as a
// typed message — the adapter decides.
type FeatureCollection struct {
	Type     string    `json:"type"`
	Features []Feature `json:"features"`
}

// Feature is a single GeoJSON Feature. Properties carries the MOFA metadata
// so the client can tooltip without a second fetch.
type Feature struct {
	Type       string     `json:"type"`
	Geometry   Geometry   `json:"geometry"`
	Properties Properties `json:"properties"`
}

// Geometry is a GeoJSON Point. The coordinates order is [lng, lat] per spec.
type Geometry struct {
	Type        string    `json:"type"`
	Coordinates []float64 `json:"coordinates"`
}

// Properties mirrors the incident identifiers the Flutter client needs.
type Properties struct {
	KeyCd       string `json:"key_cd"`
	InfoType    string `json:"info_type"`
	Title       string `json:"title"`
	CountryCd   string `json:"country_cd"`
	CountryName string `json:"country_name"`
}

// Execute reads up to filter.Limit incidents and encodes them as a
// FeatureCollection. The nextCursor is intentionally discarded; callers
// wanting paged GeoJSON should call List + encode themselves.
func (u *GeoJSONUseCase) Execute(ctx context.Context, filter domain.ListFilter) (FeatureCollection, error) {
	items, _, err := u.reader.List(ctx, filter)
	if err != nil {
		return FeatureCollection{}, err
	}
	features := make([]Feature, 0, len(items))
	for _, item := range items {
		features = append(features, Feature{
			Type: "Feature",
			Geometry: Geometry{
				Type:        "Point",
				Coordinates: []float64{item.Geometry.Lng, item.Geometry.Lat},
			},
			Properties: Properties{
				KeyCd:       item.KeyCd,
				InfoType:    item.InfoType,
				Title:       item.Title,
				CountryCd:   item.CountryCd,
				CountryName: item.CountryName,
			},
		})
	}
	return FeatureCollection{Type: "FeatureCollection", Features: features}, nil
}
