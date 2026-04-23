package domain_test

import (
	"math"
	"testing"

	"github.com/soneda-yuya/reearth-homework/internal/safetyincident/domain"
	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
	"pgregory.net/rapid"
)

func TestPoint_Validate_KnownGoodCases(t *testing.T) {
	t.Parallel()
	good := []domain.Point{
		{Lat: 0, Lng: 0},
		{Lat: 35.6762, Lng: 139.6503},  // Tokyo
		{Lat: -33.8688, Lng: 151.2093}, // Sydney
		{Lat: 90, Lng: 180},
		{Lat: -90, Lng: -180},
	}
	for _, p := range good {
		if err := p.Validate(); err != nil {
			t.Errorf("Point(%v).Validate() = %v, want nil", p, err)
		}
	}
}

func TestPoint_Validate_KnownBadCases(t *testing.T) {
	t.Parallel()
	bad := []domain.Point{
		{Lat: 90.1, Lng: 0},
		{Lat: -90.1, Lng: 0},
		{Lat: 0, Lng: 180.1},
		{Lat: 0, Lng: -180.1},
		{Lat: math.NaN(), Lng: 0},
		{Lat: 0, Lng: math.NaN()},
	}
	for _, p := range bad {
		err := p.Validate()
		if err == nil {
			t.Errorf("Point(%v).Validate() = nil, want error", p)
			continue
		}
		if !errs.IsKind(err, errs.KindInvalidInput) {
			t.Errorf("Point(%v) kind = %s, want KindInvalidInput", p, errs.KindOf(err))
		}
	}
}

// TestPoint_Validate_Property uses PBT to assert the equivalence between
// "in [-90,90] × [-180,180]" and "Validate succeeds". Coverage is asserted
// in both directions: every valid coordinate must pass, every out-of-range
// coordinate must fail.
func TestPoint_Validate_Property(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		lat := rapid.Float64Range(-90, 90).Draw(t, "lat")
		lng := rapid.Float64Range(-180, 180).Draw(t, "lng")
		if err := (domain.Point{Lat: lat, Lng: lng}).Validate(); err != nil {
			t.Fatalf("in-range Point(%v,%v) failed Validate: %v", lat, lng, err)
		}
	})
	rapid.Check(t, func(t *rapid.T) {
		// Sample lat outside the valid envelope (positive side).
		lat := rapid.Float64Range(90.0001, 1000).Draw(t, "lat_oor")
		lng := rapid.Float64Range(-180, 180).Draw(t, "lng")
		if err := (domain.Point{Lat: lat, Lng: lng}).Validate(); err == nil {
			t.Fatalf("out-of-range Point(%v,%v) should fail Validate", lat, lng)
		}
	})
	rapid.Check(t, func(t *rapid.T) {
		lat := rapid.Float64Range(-90, 90).Draw(t, "lat")
		lng := rapid.Float64Range(180.0001, 1000).Draw(t, "lng_oor")
		if err := (domain.Point{Lat: lat, Lng: lng}).Validate(); err == nil {
			t.Fatalf("out-of-range Point(%v,%v) should fail Validate", lat, lng)
		}
	})
}
