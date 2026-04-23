package domain

import "context"

// ProfileRepository is the write+read port the BFF unit uses to manipulate
// the `users/{uid}` Firestore document. All mutations are idempotent: the
// BFF doesn't track "was this the create or the update" — the Flutter app
// retries freely.
type ProfileRepository interface {
	// Get returns the profile for uid, or errs.KindNotFound if the document
	// doesn't exist. Callers (specifically GetProfileUseCase) typically
	// follow a miss with CreateIfMissing.
	Get(ctx context.Context, uid string) (*UserProfile, error)
	// CreateIfMissing writes the given profile only when the document does
	// not yet exist. Returning nil on an existing doc keeps the call
	// idempotent across retries.
	CreateIfMissing(ctx context.Context, profile UserProfile) error
	// ToggleFavoriteCountry adds countryCd to favorite_country_cds if absent,
	// removes it if present. Implemented with two Firestore ArrayUnion /
	// ArrayRemove writes is acceptable; a single transactional toggle is a
	// future optimisation.
	ToggleFavoriteCountry(ctx context.Context, uid, countryCd string) error
	// UpdateNotificationPreference replaces the entire preference object.
	UpdateNotificationPreference(ctx context.Context, uid string, pref NotificationPreference) error
	// RegisterFcmToken appends a token to fcm_tokens (ArrayUnion). Duplicates
	// are de-duped by Firestore.
	RegisterFcmToken(ctx context.Context, uid, token string) error
}

// AuthVerifier validates a Firebase ID token and returns the authenticated
// uid. Any error returned should have errs.KindUnauthorized so the rpc
// layer translates it to connect.CodeUnauthenticated. Implementations are
// expected to cache Firebase public keys and verify tokens locally (see
// Code Generation Plan Q C [A] — revocation check is not used in MVP).
type AuthVerifier interface {
	Verify(ctx context.Context, idToken string) (uid string, err error)
}
