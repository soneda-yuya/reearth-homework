// Package firestore is the BFF-side adapter onto the `users/{uid}` Firestore
// collection. All operations are document-id direct access so no composite
// index is required; ArrayUnion / ArrayRemove give idempotent list mutations.
package firestore

import (
	"context"
	"errors"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
	"github.com/soneda-yuya/overseas-safety-map/internal/user/domain"
)

// FirestoreProfileRepository implements domain.ProfileRepository against the
// `users` collection. The notification BC reads the same document through
// its own narrower struct; writes only flow through this adapter.
type FirestoreProfileRepository struct {
	client     *firestore.Client
	collection string
}

// Config configures the adapter. Collection defaults to "users".
type Config struct {
	Collection string
}

// New wires the adapter to a Firestore Client.
func New(client *firestore.Client, cfg Config) *FirestoreProfileRepository {
	if cfg.Collection == "" {
		cfg.Collection = "users"
	}
	return &FirestoreProfileRepository{client: client, collection: cfg.Collection}
}

// profileDoc is the wire shape. Field names match both
// notification/domain.UserProfile and user/domain.UserProfile so the two BCs
// share the same physical document.
type profileDoc struct {
	FavoriteCountryCds     []string `firestore:"favorite_country_cds"`
	FCMTokens              []string `firestore:"fcm_tokens"`
	NotificationPreference struct {
		Enabled          bool     `firestore:"enabled"`
		TargetCountryCds []string `firestore:"target_country_cds"`
		InfoTypes        []string `firestore:"info_types"`
	} `firestore:"notification_preference"`
}

// Get fetches the profile by uid. Missing docs surface as errs.KindNotFound.
func (r *FirestoreProfileRepository) Get(ctx context.Context, uid string) (*domain.UserProfile, error) {
	if uid == "" {
		return nil, errs.Wrap("user.firestore.get", errs.KindInvalidInput,
			errors.New("uid is required"))
	}
	snap, err := r.client.Collection(r.collection).Doc(uid).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, errs.Wrap("user.firestore.get", errs.KindNotFound, err)
		}
		return nil, errs.Wrap("user.firestore.get", errs.KindExternal, err)
	}
	var doc profileDoc
	if err := snap.DataTo(&doc); err != nil {
		return nil, errs.Wrap("user.firestore.get", errs.KindInternal, err)
	}
	return &domain.UserProfile{
		UID:                uid,
		FavoriteCountryCds: doc.FavoriteCountryCds,
		FCMTokens:          doc.FCMTokens,
		NotificationPreference: domain.NotificationPreference{
			Enabled:          doc.NotificationPreference.Enabled,
			TargetCountryCds: doc.NotificationPreference.TargetCountryCds,
			InfoTypes:        doc.NotificationPreference.InfoTypes,
		},
	}, nil
}

// CreateIfMissing uses Firestore's Create op, which fails with AlreadyExists
// when the doc is present. We swallow that error so the operation is
// idempotent — a retry after a successful first call is a no-op.
func (r *FirestoreProfileRepository) CreateIfMissing(ctx context.Context, profile domain.UserProfile) error {
	if profile.UID == "" {
		return errs.Wrap("user.firestore.create_if_missing", errs.KindInvalidInput,
			errors.New("uid is required"))
	}
	doc := profileDoc{
		FavoriteCountryCds: defaulted(profile.FavoriteCountryCds),
		FCMTokens:          defaulted(profile.FCMTokens),
	}
	doc.NotificationPreference.Enabled = profile.NotificationPreference.Enabled
	doc.NotificationPreference.TargetCountryCds = defaulted(profile.NotificationPreference.TargetCountryCds)
	doc.NotificationPreference.InfoTypes = defaulted(profile.NotificationPreference.InfoTypes)

	_, err := r.client.Collection(r.collection).Doc(profile.UID).Create(ctx, doc)
	if err != nil {
		if status.Code(err) == codes.AlreadyExists {
			return nil
		}
		return errs.Wrap("user.firestore.create_if_missing", errs.KindExternal, err)
	}
	return nil
}

// ToggleFavoriteCountry reads the current array, decides add-or-remove, then
// issues the single corresponding update. Two round-trips (read + write)
// is acceptable at MVP scale; if contention becomes an issue this can move
// into a RunTransaction.
func (r *FirestoreProfileRepository) ToggleFavoriteCountry(ctx context.Context, uid, countryCd string) error {
	if uid == "" || countryCd == "" {
		return errs.Wrap("user.firestore.toggle_favorite", errs.KindInvalidInput,
			errors.New("uid and country_cd are required"))
	}
	snap, err := r.client.Collection(r.collection).Doc(uid).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return errs.Wrap("user.firestore.toggle_favorite", errs.KindNotFound, err)
		}
		return errs.Wrap("user.firestore.toggle_favorite", errs.KindExternal, err)
	}
	var doc profileDoc
	if err := snap.DataTo(&doc); err != nil {
		return errs.Wrap("user.firestore.toggle_favorite", errs.KindInternal, err)
	}
	var update firestore.Update
	if contains(doc.FavoriteCountryCds, countryCd) {
		update = firestore.Update{Path: "favorite_country_cds", Value: firestore.ArrayRemove(countryCd)}
	} else {
		update = firestore.Update{Path: "favorite_country_cds", Value: firestore.ArrayUnion(countryCd)}
	}
	if _, err := r.client.Collection(r.collection).Doc(uid).Update(ctx, []firestore.Update{update}); err != nil {
		return errs.Wrap("user.firestore.toggle_favorite", errs.KindExternal, err)
	}
	return nil
}

// UpdateNotificationPreference overwrites the whole nested preference object.
// A single-field Update keeps the other fields untouched.
func (r *FirestoreProfileRepository) UpdateNotificationPreference(ctx context.Context, uid string, pref domain.NotificationPreference) error {
	if uid == "" {
		return errs.Wrap("user.firestore.update_preference", errs.KindInvalidInput,
			errors.New("uid is required"))
	}
	update := firestore.Update{
		Path: "notification_preference",
		Value: map[string]any{
			"enabled":            pref.Enabled,
			"target_country_cds": defaulted(pref.TargetCountryCds),
			"info_types":         defaulted(pref.InfoTypes),
		},
	}
	if _, err := r.client.Collection(r.collection).Doc(uid).Update(ctx, []firestore.Update{update}); err != nil {
		if status.Code(err) == codes.NotFound {
			return errs.Wrap("user.firestore.update_preference", errs.KindNotFound, err)
		}
		return errs.Wrap("user.firestore.update_preference", errs.KindExternal, err)
	}
	return nil
}

// RegisterFcmToken appends a token via ArrayUnion (idempotent on duplicates).
func (r *FirestoreProfileRepository) RegisterFcmToken(ctx context.Context, uid, token string) error {
	if uid == "" || token == "" {
		return errs.Wrap("user.firestore.register_fcm_token", errs.KindInvalidInput,
			errors.New("uid and fcm_token are required"))
	}
	_, err := r.client.Collection(r.collection).Doc(uid).Update(ctx, []firestore.Update{
		{Path: "fcm_tokens", Value: firestore.ArrayUnion(token)},
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return errs.Wrap("user.firestore.register_fcm_token", errs.KindNotFound, err)
		}
		return errs.Wrap("user.firestore.register_fcm_token", errs.KindExternal, err)
	}
	return nil
}

// defaulted replaces a nil slice with an empty one so Firestore stores "[]"
// rather than "null" (Firestore accepts both, but empty arrays are easier to
// reason about in security rules and in the U-NTF query path).
func defaulted(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
