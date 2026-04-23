package application

import (
	"context"
	"errors"

	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
	"github.com/soneda-yuya/overseas-safety-map/internal/user/domain"
)

// RegisterFcmTokenUseCase appends a new FCM token to the user's profile.
// Duplicates are de-duped by Firestore's ArrayUnion on the write side.
type RegisterFcmTokenUseCase struct {
	repo domain.ProfileRepository
}

// NewRegisterFcmTokenUseCase wires the repo dependency.
func NewRegisterFcmTokenUseCase(repo domain.ProfileRepository) *RegisterFcmTokenUseCase {
	return &RegisterFcmTokenUseCase{repo: repo}
}

// Execute validates inputs and delegates to the repo. An empty token is a
// client bug — reject it loudly rather than silently persist a no-op.
func (u *RegisterFcmTokenUseCase) Execute(ctx context.Context, uid, token string) error {
	if uid == "" || token == "" {
		return errs.Wrap("user.register_fcm_token", errs.KindInvalidInput,
			errors.New("uid and fcm_token are required"))
	}
	if err := u.repo.CreateIfMissing(ctx, domain.EmptyProfile(uid)); err != nil {
		return err
	}
	return u.repo.RegisterFcmToken(ctx, uid, token)
}
