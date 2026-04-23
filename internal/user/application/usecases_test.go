package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
	"github.com/soneda-yuya/overseas-safety-map/internal/user/application"
	"github.com/soneda-yuya/overseas-safety-map/internal/user/domain"
)

func TestGetProfile_LazyCreate(t *testing.T) {
	t.Parallel()
	repo := newFakeProfileRepo()
	uc := application.NewGetProfileUseCase(repo)

	got, err := uc.Execute(context.Background(), "uid-new")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.UID != "uid-new" {
		t.Errorf("lazy-created profile UID = %q; want uid-new", got.UID)
	}
}

func TestGetProfile_Existing(t *testing.T) {
	t.Parallel()
	repo := newFakeProfileRepo()
	repo.store["uid-exists"] = domain.UserProfile{
		UID:                "uid-exists",
		FavoriteCountryCds: []string{"JP"},
	}
	uc := application.NewGetProfileUseCase(repo)

	got, err := uc.Execute(context.Background(), "uid-exists")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(got.FavoriteCountryCds) != 1 || got.FavoriteCountryCds[0] != "JP" {
		t.Errorf("FavoriteCountryCds = %v; want [JP]", got.FavoriteCountryCds)
	}
}

func TestGetProfile_EmptyUID(t *testing.T) {
	t.Parallel()
	uc := application.NewGetProfileUseCase(newFakeProfileRepo())
	_, err := uc.Execute(context.Background(), "")
	if !errs.IsKind(err, errs.KindInvalidInput) {
		t.Errorf("empty uid err = %v; want KindInvalidInput", err)
	}
}

func TestGetProfile_RepoError(t *testing.T) {
	t.Parallel()
	repo := newFakeProfileRepo()
	repo.getErr = errs.Wrap("fake", errs.KindExternal, errors.New("firestore down"))
	uc := application.NewGetProfileUseCase(repo)
	if _, err := uc.Execute(context.Background(), "uid-err"); !errs.IsKind(err, errs.KindExternal) {
		t.Errorf("repo error should propagate as external; got %v", err)
	}
}

func TestToggleFavorite_AddThenRemove(t *testing.T) {
	t.Parallel()
	repo := newFakeProfileRepo()
	uc := application.NewToggleFavoriteCountryUseCase(repo)
	ctx := context.Background()

	if err := uc.Execute(ctx, "uid-1", "JP"); err != nil {
		t.Fatalf("first toggle: %v", err)
	}
	if got := repo.store["uid-1"].FavoriteCountryCds; len(got) != 1 || got[0] != "JP" {
		t.Errorf("after add: %v", got)
	}
	if err := uc.Execute(ctx, "uid-1", "JP"); err != nil {
		t.Fatalf("second toggle: %v", err)
	}
	if got := repo.store["uid-1"].FavoriteCountryCds; len(got) != 0 {
		t.Errorf("after remove: %v; want empty", got)
	}
}

func TestToggleFavorite_MissingInputs(t *testing.T) {
	t.Parallel()
	uc := application.NewToggleFavoriteCountryUseCase(newFakeProfileRepo())
	cases := []struct{ uid, cc string }{{"", "JP"}, {"uid", ""}, {"", ""}}
	for _, tc := range cases {
		if err := uc.Execute(context.Background(), tc.uid, tc.cc); !errs.IsKind(err, errs.KindInvalidInput) {
			t.Errorf("Execute(%q,%q) err = %v; want KindInvalidInput", tc.uid, tc.cc, err)
		}
	}
}

func TestUpdatePreference(t *testing.T) {
	t.Parallel()
	repo := newFakeProfileRepo()
	uc := application.NewUpdateNotificationPreferenceUseCase(repo)

	pref := domain.NotificationPreference{
		Enabled:          true,
		TargetCountryCds: []string{"JP", "US"},
		InfoTypes:        []string{"spot_info"},
	}
	if err := uc.Execute(context.Background(), "uid-1", pref); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := repo.store["uid-1"].NotificationPreference
	if !got.Enabled || len(got.TargetCountryCds) != 2 || got.InfoTypes[0] != "spot_info" {
		t.Errorf("preference not persisted: %+v", got)
	}
}

func TestUpdatePreference_EmptyUID(t *testing.T) {
	t.Parallel()
	uc := application.NewUpdateNotificationPreferenceUseCase(newFakeProfileRepo())
	if err := uc.Execute(context.Background(), "", domain.NotificationPreference{}); !errs.IsKind(err, errs.KindInvalidInput) {
		t.Errorf("empty uid err = %v; want KindInvalidInput", err)
	}
}

func TestRegisterFcmToken(t *testing.T) {
	t.Parallel()
	repo := newFakeProfileRepo()
	uc := application.NewRegisterFcmTokenUseCase(repo)

	if err := uc.Execute(context.Background(), "uid-1", "tok-A"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Re-registering the same token should be a no-op (idempotent ArrayUnion).
	if err := uc.Execute(context.Background(), "uid-1", "tok-A"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := repo.store["uid-1"].FCMTokens; len(got) != 1 || got[0] != "tok-A" {
		t.Errorf("FCMTokens = %v; want [tok-A]", got)
	}
}

func TestRegisterFcmToken_MissingInputs(t *testing.T) {
	t.Parallel()
	uc := application.NewRegisterFcmTokenUseCase(newFakeProfileRepo())
	for _, tc := range []struct{ uid, tok string }{{"", "t"}, {"uid", ""}} {
		if err := uc.Execute(context.Background(), tc.uid, tc.tok); !errs.IsKind(err, errs.KindInvalidInput) {
			t.Errorf("Execute(%q,%q) err = %v; want KindInvalidInput", tc.uid, tc.tok, err)
		}
	}
}
