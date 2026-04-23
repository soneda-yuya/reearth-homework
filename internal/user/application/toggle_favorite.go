package application

import (
	"context"
	"errors"

	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
	"github.com/soneda-yuya/overseas-safety-map/internal/user/domain"
)

// ToggleFavoriteCountryUseCase flips the membership of countryCd in the
// user's favorite_country_cds array. The action is idempotent: toggling an
// unknown uid implicitly creates an empty profile first.
type ToggleFavoriteCountryUseCase struct {
	repo domain.ProfileRepository
}

// NewToggleFavoriteCountryUseCase wires the repo dependency.
func NewToggleFavoriteCountryUseCase(repo domain.ProfileRepository) *ToggleFavoriteCountryUseCase {
	return &ToggleFavoriteCountryUseCase{repo: repo}
}

// Execute runs the toggle. Empty inputs are rejected at the boundary so the
// repository layer can trust its args.
func (u *ToggleFavoriteCountryUseCase) Execute(ctx context.Context, uid, countryCd string) error {
	if uid == "" || countryCd == "" {
		return errs.Wrap("user.toggle_favorite", errs.KindInvalidInput,
			errors.New("uid and country_cd are required"))
	}
	if err := u.repo.CreateIfMissing(ctx, domain.EmptyProfile(uid)); err != nil {
		return err
	}
	return u.repo.ToggleFavoriteCountry(ctx, uid, countryCd)
}
