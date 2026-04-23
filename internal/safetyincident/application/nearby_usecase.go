package application

import (
	"context"

	"github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/domain"
)

// NearbyUseCase returns incidents within radiusKm of center, bounded by limit.
type NearbyUseCase struct {
	reader domain.SafetyIncidentReader
}

// NewNearbyUseCase wires the reader dependency.
func NewNearbyUseCase(reader domain.SafetyIncidentReader) *NearbyUseCase {
	return &NearbyUseCase{reader: reader}
}

// Execute forwards to the reader. The proximity query is a top-N (no cursor)
// by design — see domain.SafetyIncidentReader.ListNearby for rationale.
func (u *NearbyUseCase) Execute(ctx context.Context, center domain.Point, radiusKm float64, limit int) ([]domain.SafetyIncident, error) {
	return u.reader.ListNearby(ctx, center, radiusKm, limit)
}
