// Package userrepo is the notifier's read-only-ish view onto the `users`
// Firestore collection. "Read-only-ish" because the only write we perform
// is ArrayRemove on fcm_tokens, which happens when FCM declares a token
// permanently dead.
package userrepo

import (
	"context"
	"errors"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"

	"github.com/soneda-yuya/overseas-safety-map/internal/notification/domain"
	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
)

// FirestoreUserRepository implements domain.UserRepository.
type FirestoreUserRepository struct {
	client     *firestore.Client
	collection string
}

// Config configures the adapter. Collection defaults to "users" (U-BFF owns
// the collection; this adapter reads + ArrayRemoves fcm_tokens only).
type Config struct {
	Collection string
}

// New wires the adapter to a Firestore Client.
func New(client *firestore.Client, cfg Config) *FirestoreUserRepository {
	if cfg.Collection == "" {
		cfg.Collection = "users"
	}
	return &FirestoreUserRepository{client: client, collection: cfg.Collection}
}

// userDoc is the wire shape we decode from Firestore. Nested struct tags
// follow dotted field paths used in the composite index.
type userDoc struct {
	FCMTokens              []string `firestore:"fcm_tokens"`
	NotificationPreference struct {
		Enabled          bool     `firestore:"enabled"`
		TargetCountryCds []string `firestore:"target_country_cds"`
		InfoTypes        []string `firestore:"info_types"`
	} `firestore:"notification_preference"`
}

// FindSubscribers queries Firestore with the composite index defined in
// Terraform, then applies the info_types filter in memory (Firestore only
// supports one array-contains per query).
func (r *FirestoreUserRepository) FindSubscribers(ctx context.Context, countryCd, infoType string) ([]domain.UserProfile, error) {
	it := r.client.Collection(r.collection).
		Where("notification_preference.enabled", "==", true).
		Where("notification_preference.target_country_cds", "array-contains", countryCd).
		Documents(ctx)
	defer it.Stop()

	var out []domain.UserProfile
	for {
		doc, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, errs.Wrap("userrepo.firestore", errs.KindExternal, err)
		}

		var u userDoc
		if err := doc.DataTo(&u); err != nil {
			return nil, errs.Wrap("userrepo.decode", errs.KindInternal, err)
		}
		pref := domain.NotificationPreference{
			Enabled:          u.NotificationPreference.Enabled,
			TargetCountryCds: u.NotificationPreference.TargetCountryCds,
			InfoTypes:        u.NotificationPreference.InfoTypes,
		}
		if !pref.WantsInfoType(infoType) {
			continue
		}
		if len(u.FCMTokens) == 0 {
			continue
		}
		out = append(out, domain.UserProfile{
			UID:                    doc.Ref.ID,
			FCMTokens:              u.FCMTokens,
			NotificationPreference: pref,
		})
	}
	return out, nil
}

// RemoveInvalidTokens strips the given tokens from the user's fcm_tokens
// array. The operation is transactional on the server side; concurrent
// writes from U-BFF (adding new tokens) are safe.
func (r *FirestoreUserRepository) RemoveInvalidTokens(ctx context.Context, uid string, tokens []string) error {
	if len(tokens) == 0 {
		return nil
	}
	// firestore.ArrayRemove takes variadic interface{}; cast through []any.
	values := make([]interface{}, len(tokens))
	for i, t := range tokens {
		values[i] = t
	}
	_, err := r.client.Collection(r.collection).Doc(uid).Update(ctx, []firestore.Update{{
		Path:  "fcm_tokens",
		Value: firestore.ArrayRemove(values...),
	}})
	if err != nil {
		return errs.Wrap("userrepo.array_remove", errs.KindExternal, err)
	}
	return nil
}
