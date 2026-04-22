// Package validate provides request-level validation helpers used by Connect
// handlers and job runners before any business logic runs.
//
// All functions return errors wrapped with [errs.KindInvalidInput] so the
// Connect interceptor can map them to InvalidArgument automatically.
package validate

import (
	"fmt"
	"time"

	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

// NonEmpty returns an error if v is empty.
func NonEmpty(field, v string) error {
	if v == "" {
		return errs.Wrap("validate.nonempty", errs.KindInvalidInput,
			fmt.Errorf("%s must not be empty", field))
	}
	return nil
}

// IntRange returns an error if v is outside [min, max] inclusive.
func IntRange(field string, v, minV, maxV int) error {
	if v < minV || v > maxV {
		return errs.Wrap("validate.intrange", errs.KindInvalidInput,
			fmt.Errorf("%s must be in [%d, %d], got %d", field, minV, maxV, v))
	}
	return nil
}

// Float64Range returns an error if v is outside [min, max] inclusive.
func Float64Range(field string, v, minV, maxV float64) error {
	if v < minV || v > maxV {
		return errs.Wrap("validate.floatrange", errs.KindInvalidInput,
			fmt.Errorf("%s must be in [%.6f, %.6f], got %.6f", field, minV, maxV, v))
	}
	return nil
}

// DurationOrder returns an error if from is after to (both non-zero).
// Zero values on either side are treated as "open-ended" and are always valid.
func DurationOrder(fromField, toField string, from, to time.Time) error {
	if from.IsZero() || to.IsZero() {
		return nil
	}
	if from.After(to) {
		return errs.Wrap("validate.order", errs.KindInvalidInput,
			fmt.Errorf("%s (%s) must be <= %s (%s)",
				fromField, from.Format(time.RFC3339),
				toField, to.Format(time.RFC3339)))
	}
	return nil
}

// LatLng returns an error if the coordinate is outside the WGS84 valid range.
func LatLng(lat, lng float64) error {
	if err := Float64Range("lat", lat, -90, 90); err != nil {
		return err
	}
	return Float64Range("lng", lng, -180, 180)
}
