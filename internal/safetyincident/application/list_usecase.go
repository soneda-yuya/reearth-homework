package application

import (
	"context"

	"github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/domain"
)

// ListUseCase returns a filtered, cursor-paginated slice of incidents. It is a
// thin wrapper over the reader — the BFF does not own any list-time business
// logic. Kept as a struct (not a free function) so future cross-cutting
// concerns like rate limiting can be added without changing call sites.
type ListUseCase struct {
	reader domain.SafetyIncidentReader
}

// NewListUseCase wires the reader dependency.
func NewListUseCase(reader domain.SafetyIncidentReader) *ListUseCase {
	return &ListUseCase{reader: reader}
}

// Execute forwards to the reader. The nextCursor is opaque at this layer and
// passed straight through to the RPC response.
func (u *ListUseCase) Execute(ctx context.Context, filter domain.ListFilter) (items []domain.SafetyIncident, nextCursor string, err error) {
	return u.reader.List(ctx, filter)
}
