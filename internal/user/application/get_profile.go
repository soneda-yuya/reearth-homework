// Package application holds the user BC use cases that the BFF RPC handlers
// invoke. All writes are idempotent so the Flutter app can retry freely.
package application

import (
	"context"
	"errors"

	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
	"github.com/soneda-yuya/overseas-safety-map/internal/user/domain"
)

// GetProfileUseCase reads the profile for the authenticated uid. On first
// call for a brand-new uid, it lazily creates an empty profile so the
// Flutter client never has to distinguish "new user" from "existing user"
// at the UX layer.
type GetProfileUseCase struct {
	repo domain.ProfileRepository
}

// NewGetProfileUseCase wires the repo dependency.
func NewGetProfileUseCase(repo domain.ProfileRepository) *GetProfileUseCase {
	return &GetProfileUseCase{repo: repo}
}

// Execute returns the profile. If it does not exist yet, Execute creates an
// empty one and returns it — this single RPC therefore doubles as the
// account bootstrap hook.
func (u *GetProfileUseCase) Execute(ctx context.Context, uid string) (*domain.UserProfile, error) {
	if uid == "" {
		return nil, errs.Wrap("user.get_profile", errs.KindInvalidInput, errors.New("uid is required"))
	}
	profile, err := u.repo.Get(ctx, uid)
	if err == nil {
		return profile, nil
	}
	if !errs.IsKind(err, errs.KindNotFound) {
		return nil, err
	}
	empty := domain.EmptyProfile(uid)
	if createErr := u.repo.CreateIfMissing(ctx, empty); createErr != nil {
		return nil, createErr
	}
	// Re-fetch to pick up server-side defaults (e.g. timestamps).
	return u.repo.Get(ctx, uid)
}
