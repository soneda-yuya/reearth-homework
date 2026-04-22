package validate_test

import (
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
	"github.com/soneda-yuya/reearth-homework/internal/shared/validate"
)

func TestNonEmpty(t *testing.T) {
	if err := validate.NonEmpty("field", ""); err == nil {
		t.Fatal("empty string should fail")
	} else if !errs.IsKind(err, errs.KindInvalidInput) {
		t.Fatalf("want KindInvalidInput, got %q", errs.KindOf(err))
	}
	if err := validate.NonEmpty("field", "x"); err != nil {
		t.Fatalf("non-empty should pass: %v", err)
	}
}

func TestIntRange_Inclusive(t *testing.T) {
	if err := validate.IntRange("limit", 1, 1, 100); err != nil {
		t.Errorf("boundary min should pass: %v", err)
	}
	if err := validate.IntRange("limit", 100, 1, 100); err != nil {
		t.Errorf("boundary max should pass: %v", err)
	}
	if err := validate.IntRange("limit", 0, 1, 100); err == nil {
		t.Errorf("below range should fail")
	}
	if err := validate.IntRange("limit", 101, 1, 100); err == nil {
		t.Errorf("above range should fail")
	}
}

func TestDurationOrder(t *testing.T) {
	a := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	b := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	if err := validate.DurationOrder("from", "to", a, b); err != nil {
		t.Errorf("a <= b should pass: %v", err)
	}
	if err := validate.DurationOrder("from", "to", b, a); err == nil {
		t.Errorf("a > b should fail")
	}
	if err := validate.DurationOrder("from", "to", time.Time{}, b); err != nil {
		t.Errorf("zero from should pass (open-ended)")
	}
	if err := validate.DurationOrder("from", "to", a, time.Time{}); err != nil {
		t.Errorf("zero to should pass (open-ended)")
	}
}

func TestLatLng_Boundaries(t *testing.T) {
	ok := [][2]float64{{0, 0}, {-90, -180}, {90, 180}, {45, 135}}
	for _, p := range ok {
		if err := validate.LatLng(p[0], p[1]); err != nil {
			t.Errorf("LatLng(%v, %v) should pass: %v", p[0], p[1], err)
		}
	}
	bad := [][2]float64{{-90.01, 0}, {90.01, 0}, {0, -180.01}, {0, 180.01}}
	for _, p := range bad {
		if err := validate.LatLng(p[0], p[1]); err == nil {
			t.Errorf("LatLng(%v, %v) should fail", p[0], p[1])
		}
	}
}

// Property: any coordinate inside the WGS84 box validates successfully and any
// value strictly outside either axis fails.
func TestProp_LatLngBox(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		lat := rapid.Float64Range(-90, 90).Draw(t, "lat")
		lng := rapid.Float64Range(-180, 180).Draw(t, "lng")
		if err := validate.LatLng(lat, lng); err != nil {
			t.Fatalf("inside-box should pass: lat=%v lng=%v err=%v", lat, lng, err)
		}
	})
	rapid.Check(t, func(t *rapid.T) {
		// Generate a latitude outside the valid range deliberately.
		lat := rapid.Float64Range(90.0001, 1e6).Draw(t, "lat-out")
		sign := rapid.IntRange(0, 1).Draw(t, "sign")
		if sign == 1 {
			lat = -lat
		}
		if err := validate.LatLng(lat, 0); err == nil {
			t.Fatalf("out-of-range latitude %v should fail", lat)
		}
	})
}

// Property: IntRange returns nil iff min <= v <= max.
func TestProp_IntRangeMatchesDefinition(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		minV := rapid.IntRange(-1_000_000, 1_000_000).Draw(t, "min")
		delta := rapid.IntRange(0, 1_000_000).Draw(t, "delta")
		maxV := minV + delta
		v := rapid.IntRange(minV-100, maxV+100).Draw(t, "v")
		err := validate.IntRange("f", v, minV, maxV)
		inRange := v >= minV && v <= maxV
		if inRange && err != nil {
			t.Fatalf("v=%d in [%d,%d] but got err %v", v, minV, maxV, err)
		}
		if !inRange && err == nil {
			t.Fatalf("v=%d outside [%d,%d] but err was nil", v, minV, maxV)
		}
	})
}
