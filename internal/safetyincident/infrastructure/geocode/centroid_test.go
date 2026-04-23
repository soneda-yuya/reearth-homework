package geocode_test

import (
	"testing"

	"github.com/soneda-yuya/reearth-homework/internal/safetyincident/infrastructure/geocode"
	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

func TestCentroidGeocoder_LoadAndLookup(t *testing.T) {
	t.Parallel()
	g, err := geocode.NewCentroidGeocoder()
	if err != nil {
		t.Fatalf("NewCentroidGeocoder: %v", err)
	}
	if g.Size() < 100 {
		t.Errorf("Size = %d, want >= 100 (data file looks truncated)", g.Size())
	}

	// Spot-check a few well-known countries.
	cases := []struct {
		cc     string
		minLat float64
		maxLat float64
		minLng float64
		maxLng float64
		hint   string
	}{
		{cc: "JP", minLat: 30, maxLat: 46, minLng: 130, maxLng: 146, hint: "Japan"},
		{cc: "FR", minLat: 41, maxLat: 51, minLng: -5, maxLng: 10, hint: "France"},
		{cc: "US", minLat: 25, maxLat: 50, minLng: -125, maxLng: -65, hint: "United States"},
		{cc: "id", minLat: -11, maxLat: 6, minLng: 95, maxLng: 141, hint: "Indonesia (lowercase input)"},
	}
	for _, tc := range cases {
		p, err := g.Lookup(tc.cc)
		if err != nil {
			t.Errorf("%s: %v", tc.hint, err)
			continue
		}
		if p.Lat < tc.minLat || p.Lat > tc.maxLat {
			t.Errorf("%s lat = %v, want in [%v,%v]", tc.hint, p.Lat, tc.minLat, tc.maxLat)
		}
		if p.Lng < tc.minLng || p.Lng > tc.maxLng {
			t.Errorf("%s lng = %v, want in [%v,%v]", tc.hint, p.Lng, tc.minLng, tc.maxLng)
		}
	}
}

func TestCentroidGeocoder_UnknownCountry_ReturnsKindNotFound(t *testing.T) {
	t.Parallel()
	g, err := geocode.NewCentroidGeocoder()
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	_, err = g.Lookup("XX")
	if err == nil {
		t.Fatal("expected error for unknown country")
	}
	if !errs.IsKind(err, errs.KindNotFound) {
		t.Errorf("kind = %s, want KindNotFound", errs.KindOf(err))
	}
}

func TestCentroidGeocoder_EmptyCountry_ReturnsKindInvalidInput(t *testing.T) {
	t.Parallel()
	g, err := geocode.NewCentroidGeocoder()
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	_, err = g.Lookup("")
	if err == nil {
		t.Fatal("expected error for empty country")
	}
	if !errs.IsKind(err, errs.KindInvalidInput) {
		t.Errorf("kind = %s, want KindInvalidInput", errs.KindOf(err))
	}
}
