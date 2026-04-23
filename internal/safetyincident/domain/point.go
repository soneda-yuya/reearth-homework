package domain

import (
	"fmt"

	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

// Point is a WGS84 coordinate. Both values are decimal degrees.
type Point struct {
	Lat float64
	Lng float64
}

// Validate rejects coordinates outside the WGS84 envelope. The check is
// intentionally strict — silent NaN / Inf or out-of-range values would
// produce malformed CMS items.
func (p Point) Validate() error {
	switch {
	case p.Lat != p.Lat || p.Lng != p.Lng: // NaN catches itself
		return errs.Wrap("point.validate", errs.KindInvalidInput, fmt.Errorf("lat/lng is NaN"))
	case p.Lat < -90 || p.Lat > 90:
		return errs.Wrap("point.validate", errs.KindInvalidInput, fmt.Errorf("lat=%v outside [-90,90]", p.Lat))
	case p.Lng < -180 || p.Lng > 180:
		return errs.Wrap("point.validate", errs.KindInvalidInput, fmt.Errorf("lng=%v outside [-180,180]", p.Lng))
	}
	return nil
}
