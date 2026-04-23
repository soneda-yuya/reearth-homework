package domain_test

import (
	"testing"

	"github.com/soneda-yuya/reearth-homework/internal/notification/domain"
)

func TestNotificationPreference_WantsInfoType(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		types    []string
		incoming string
		want     bool
	}{
		{"empty means all", nil, "danger", true},
		{"match", []string{"danger", "spot"}, "danger", true},
		{"miss", []string{"danger"}, "spot", false},
		{"empty string in filter", []string{""}, "", true},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := domain.NotificationPreference{InfoTypes: tc.types}
			if got := p.WantsInfoType(tc.incoming); got != tc.want {
				t.Errorf("WantsInfoType(%q) = %v, want %v", tc.incoming, got, tc.want)
			}
		})
	}
}

func TestUserProfile_IsDeliverable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		user domain.UserProfile
		want bool
	}{
		{
			name: "enabled with tokens",
			user: domain.UserProfile{
				FCMTokens:              []string{"tok"},
				NotificationPreference: domain.NotificationPreference{Enabled: true},
			},
			want: true,
		},
		{
			name: "disabled",
			user: domain.UserProfile{
				FCMTokens:              []string{"tok"},
				NotificationPreference: domain.NotificationPreference{Enabled: false},
			},
			want: false,
		},
		{
			name: "enabled but no tokens",
			user: domain.UserProfile{
				NotificationPreference: domain.NotificationPreference{Enabled: true},
			},
			want: false,
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.user.IsDeliverable(); got != tc.want {
				t.Errorf("IsDeliverable() = %v, want %v", got, tc.want)
			}
		})
	}
}
