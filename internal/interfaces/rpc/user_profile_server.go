package rpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	overseasmapv1 "github.com/soneda-yuya/overseas-safety-map/gen/go/v1"
	"github.com/soneda-yuya/overseas-safety-map/internal/shared/authctx"
	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
	"github.com/soneda-yuya/overseas-safety-map/internal/user/application"
)

// UserProfileServer implements UserProfileServiceHandler. Each RPC resolves
// the caller's uid from authctx; the AuthInterceptor is responsible for
// populating it.
type UserProfileServer struct {
	getProfile       *application.GetProfileUseCase
	toggleFavorite   *application.ToggleFavoriteCountryUseCase
	updatePreference *application.UpdateNotificationPreferenceUseCase
	registerFcmToken *application.RegisterFcmTokenUseCase
}

// NewUserProfileServer wires the use case dependencies.
func NewUserProfileServer(
	getProfile *application.GetProfileUseCase,
	toggleFavorite *application.ToggleFavoriteCountryUseCase,
	updatePreference *application.UpdateNotificationPreferenceUseCase,
	registerFcmToken *application.RegisterFcmTokenUseCase,
) *UserProfileServer {
	return &UserProfileServer{
		getProfile:       getProfile,
		toggleFavorite:   toggleFavorite,
		updatePreference: updatePreference,
		registerFcmToken: registerFcmToken,
	}
}

// uidFromCtx returns the authenticated uid or an Unauthorized error. A
// missing uid means AuthInterceptor did not run (wiring bug) or it ran but
// failed to populate the ctx — either way it's safer to deny than default.
func uidFromCtx(ctx context.Context) (string, error) {
	uid, ok := authctx.UIDFrom(ctx)
	if !ok {
		return "", errs.Wrap("rpc.user.uid_missing", errs.KindUnauthorized,
			errors.New("no uid on context"))
	}
	return uid, nil
}

// GetProfile returns the caller's profile, creating an empty one on first access.
func (s *UserProfileServer) GetProfile(
	ctx context.Context,
	_ *connect.Request[overseasmapv1.GetProfileRequest],
) (*connect.Response[overseasmapv1.GetProfileResponse], error) {
	uid, err := uidFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	profile, err := s.getProfile.Execute(ctx, uid)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&overseasmapv1.GetProfileResponse{Profile: userProfileToProto(profile)}), nil
}

// ToggleFavoriteCountry flips country_cd's membership on favorite_country_cds.
func (s *UserProfileServer) ToggleFavoriteCountry(
	ctx context.Context,
	req *connect.Request[overseasmapv1.ToggleFavoriteCountryRequest],
) (*connect.Response[overseasmapv1.ToggleFavoriteCountryResponse], error) {
	uid, err := uidFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.toggleFavorite.Execute(ctx, uid, req.Msg.GetCountryCd()); err != nil {
		return nil, err
	}
	profile, err := s.getProfile.Execute(ctx, uid)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&overseasmapv1.ToggleFavoriteCountryResponse{
		Profile: userProfileToProto(profile),
	}), nil
}

// UpdateNotificationPreference replaces the whole preference object.
func (s *UserProfileServer) UpdateNotificationPreference(
	ctx context.Context,
	req *connect.Request[overseasmapv1.UpdateNotificationPreferenceRequest],
) (*connect.Response[overseasmapv1.UpdateNotificationPreferenceResponse], error) {
	uid, err := uidFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.updatePreference.Execute(ctx, uid, notificationPrefFromProto(req.Msg.GetPreference())); err != nil {
		return nil, err
	}
	return connect.NewResponse(&overseasmapv1.UpdateNotificationPreferenceResponse{}), nil
}

// RegisterFcmToken appends an FCM token (idempotent on duplicates).
func (s *UserProfileServer) RegisterFcmToken(
	ctx context.Context,
	req *connect.Request[overseasmapv1.RegisterFcmTokenRequest],
) (*connect.Response[overseasmapv1.RegisterFcmTokenResponse], error) {
	uid, err := uidFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.registerFcmToken.Execute(ctx, uid, req.Msg.GetToken()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&overseasmapv1.RegisterFcmTokenResponse{}), nil
}
