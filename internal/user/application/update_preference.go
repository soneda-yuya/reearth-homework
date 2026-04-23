package application

import (
	"context"
	"errors"

	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
	"github.com/soneda-yuya/overseas-safety-map/internal/user/domain"
)

// UpdateNotificationPreferenceUseCase replaces the entire preference object
// on the user's profile. It bootstraps a missing profile first, keeping the
// Flutter client's "save preferences" flow a single RPC.
type UpdateNotificationPreferenceUseCase struct {
	repo domain.ProfileRepository
}

// NewUpdateNotificationPreferenceUseCase wires the repo dependency.
func NewUpdateNotificationPreferenceUseCase(repo domain.ProfileRepository) *UpdateNotificationPreferenceUseCase {
	return &UpdateNotificationPreferenceUseCase{repo: repo}
}

// Execute validates uid and delegates to the repo. The preference object
// itself is not validated here — a user may legitimately save
// "enabled=false, zero countries" to disable notifications globally.
func (u *UpdateNotificationPreferenceUseCase) Execute(ctx context.Context, uid string, pref domain.NotificationPreference) error {
	if uid == "" {
		return errs.Wrap("user.update_preference", errs.KindInvalidInput,
			errors.New("uid is required"))
	}
	if err := u.repo.CreateIfMissing(ctx, domain.EmptyProfile(uid)); err != nil {
		return err
	}
	return u.repo.UpdateNotificationPreference(ctx, uid, pref)
}
