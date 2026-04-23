package application

import (
	"context"

	"github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/domain"
)

// SearchUseCase performs keyword lookup over the incident corpus.
type SearchUseCase struct {
	reader domain.SafetyIncidentReader
}

// NewSearchUseCase wires the reader dependency.
func NewSearchUseCase(reader domain.SafetyIncidentReader) *SearchUseCase {
	return &SearchUseCase{reader: reader}
}

// Execute forwards to the reader, including opaque cursor pass-through.
func (u *SearchUseCase) Execute(ctx context.Context, filter domain.SearchFilter) (items []domain.SafetyIncident, nextCursor string, err error) {
	return u.reader.Search(ctx, filter)
}
