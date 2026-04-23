package domain_test

import (
	"testing"
	"time"

	"github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/domain"
)

func TestBuild_PreservesAllSources(t *testing.T) {
	t.Parallel()
	item := validMailItem()
	extract := domain.ExtractResult{Location: "東京都新宿区", Confidence: 0.9}
	geocode := domain.GeocodeResult{
		Point:  domain.Point{Lat: 35.6938, Lng: 139.7036},
		Source: domain.GeocodeSourceMapbox,
	}
	now := time.Date(2026, 4, 23, 10, 0, 0, 0, time.UTC)

	got := domain.Build(item, extract, geocode, now)

	if got.KeyCd != item.KeyCd {
		t.Errorf("KeyCd lost: %q != %q", got.KeyCd, item.KeyCd)
	}
	if got.ExtractedLocation != "東京都新宿区" {
		t.Errorf("ExtractedLocation = %q", got.ExtractedLocation)
	}
	if got.Geometry != geocode.Point {
		t.Errorf("Geometry = %v, want %v", got.Geometry, geocode.Point)
	}
	if got.GeocodeSource != domain.GeocodeSourceMapbox {
		t.Errorf("GeocodeSource = %s, want mapbox", got.GeocodeSource)
	}
	if !got.IngestedAt.Equal(now) || !got.UpdatedAt.Equal(now) {
		t.Errorf("timestamps not pinned to now: ingested=%v updated=%v", got.IngestedAt, got.UpdatedAt)
	}
}

func TestBuild_EmptyExtractKeepsCentroidSource(t *testing.T) {
	t.Parallel()
	// When the LLM cannot extract a location, Geocoder returns a centroid
	// result. Build should propagate that source untouched so downstream UI
	// can label the pin as approximate.
	item := validMailItem()
	extract := domain.ExtractResult{Location: "", Confidence: 0}
	geocode := domain.GeocodeResult{
		Point:  domain.Point{Lat: 36, Lng: 138}, // a Japan-ish centroid
		Source: domain.GeocodeSourceCountryCentroid,
	}
	now := time.Now()

	got := domain.Build(item, extract, geocode, now)
	if got.ExtractedLocation != "" {
		t.Errorf("ExtractedLocation should remain empty, got %q", got.ExtractedLocation)
	}
	if got.GeocodeSource != domain.GeocodeSourceCountryCentroid {
		t.Errorf("GeocodeSource = %s, want country_centroid", got.GeocodeSource)
	}
}

func TestGeocodeSource_String(t *testing.T) {
	t.Parallel()
	tests := []struct {
		s    domain.GeocodeSource
		want string
	}{
		{domain.GeocodeSourceMapbox, "mapbox"},
		{domain.GeocodeSourceCountryCentroid, "country_centroid"},
	}
	for _, tc := range tests {
		if got := tc.s.String(); got != tc.want {
			t.Errorf("GeocodeSource(%d).String() = %q, want %q", int(tc.s), got, tc.want)
		}
	}
	if got := domain.GeocodeSourceUnspecified.String(); got == "" {
		t.Error("Unspecified.String() should not be empty")
	}
}

func TestGeocodeSource_IsValid(t *testing.T) {
	t.Parallel()
	for _, s := range []domain.GeocodeSource{domain.GeocodeSourceMapbox, domain.GeocodeSourceCountryCentroid} {
		if !s.IsValid() {
			t.Errorf("%v should be valid", s)
		}
	}
	for _, s := range []domain.GeocodeSource{domain.GeocodeSourceUnspecified, domain.GeocodeSource(99)} {
		if s.IsValid() {
			t.Errorf("%v should be invalid", s)
		}
	}
}
