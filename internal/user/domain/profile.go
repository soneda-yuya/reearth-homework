// Package domain holds the user bounded context: the aggregate backing the
// Firestore `users/{uid}` document and the ports the BFF unit uses to
// manipulate it. The notification BC reads the same document through its
// own narrower struct; both shapes must stay in sync with the proto
// UserProfile definition.
package domain

// UserProfile is the full user aggregate owned by the BFF. The notification
// BC reads only a subset of these fields (FCMTokens + NotificationPreference)
// via its own struct in internal/notification/domain.
type UserProfile struct {
	UID                    string
	FavoriteCountryCds     []string
	NotificationPreference NotificationPreference
	FCMTokens              []string
}

// NotificationPreference mirrors the nested object on the user doc. The
// shape must match notification/domain.NotificationPreference — callers rely
// on both structs marshalling to the same Firestore representation.
type NotificationPreference struct {
	Enabled          bool
	TargetCountryCds []string
	InfoTypes        []string
}

// EmptyProfile builds the zero-valued profile we lazily create on first
// GetProfile call for a uid. Keeping the allocator here lets the Firestore
// adapter stay free of domain-shape knowledge.
func EmptyProfile(uid string) UserProfile {
	return UserProfile{
		UID:                    uid,
		FavoriteCountryCds:     []string{},
		FCMTokens:              []string{},
		NotificationPreference: NotificationPreference{},
	}
}
