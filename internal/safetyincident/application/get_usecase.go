package application

import (
	"context"

	"github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/domain"
)

// GetUseCase fetches a single incident by MOFA key_cd.
type GetUseCase struct {
	reader domain.SafetyIncidentReader
}

// NewGetUseCase wires the reader dependency.
func NewGetUseCase(reader domain.SafetyIncidentReader) *GetUseCase {
	return &GetUseCase{reader: reader}
}

// Execute forwards to the reader. Missing items surface as errs.KindNotFound
// from the reader and are translated to connect.CodeNotFound by the RPC
// error interceptor.
func (u *GetUseCase) Execute(ctx context.Context, keyCd string) (*domain.SafetyIncident, error) {
	return u.reader.Get(ctx, keyCd)
}
