package domain_test

import (
	"testing"

	"github.com/soneda-yuya/overseas-safety-map/internal/user/domain"
)

func TestEmptyProfile(t *testing.T) {
	t.Parallel()
	got := domain.EmptyProfile("uid-123")
	if got.UID != "uid-123" {
		t.Errorf("UID = %q; want uid-123", got.UID)
	}
	if got.FavoriteCountryCds == nil {
		t.Error("FavoriteCountryCds must be an empty slice, not nil (Firestore rejects nil arrays)")
	}
	if got.FCMTokens == nil {
		t.Error("FCMTokens must be an empty slice, not nil")
	}
	if got.NotificationPreference.Enabled {
		t.Error("new profile must have notifications disabled until user opts in")
	}
}
