// Package dedup is the notifier idempotency adapter. It owns the
// `notifier_dedup` Firestore collection and a 24h TTL marker so replayed
// Pub/Sub messages short-circuit at the use case boundary.
package dedup

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

// FirestoreDedup is the concrete domain.Dedup backed by Firestore.
//
// The TTL policy (`google_firestore_field.notifier_dedup_ttl`) is declared
// in Terraform; this adapter writes `expireAt = now() + ttl` so Firestore's
// automatic TTL mechanism reaps old markers within 24h (typically much
// sooner, but Google documents a worst-case 24h lag).
type FirestoreDedup struct {
	client     *firestore.Client
	collection string
	ttl        time.Duration
}

// Config configures the dedup adapter. Collection defaults to
// "notifier_dedup" (aligned with Terraform) and TTL to 24h.
type Config struct {
	Collection string
	TTL        time.Duration
}

// New returns a FirestoreDedup. client is the caller-owned Firestore Client
// (the notifier composition root closes it at shutdown).
func New(client *firestore.Client, cfg Config) *FirestoreDedup {
	if cfg.Collection == "" {
		cfg.Collection = "notifier_dedup"
	}
	if cfg.TTL == 0 {
		cfg.TTL = 24 * time.Hour
	}
	return &FirestoreDedup{client: client, collection: cfg.Collection, ttl: cfg.TTL}
}

// CheckAndMark implements domain.Dedup atomically: read → create if missing
// → return (false, nil). Two concurrent requests on the same key_cd produce
// exactly one (false, nil) and one (true, nil) because Firestore serialises
// transaction writes on the same document.
//
// Firestore may retry the transaction function on conflict, so the closure
// resets `seen` on every attempt into a local variable; the outer return
// value is only assigned after RunTransaction returns success, which
// prevents a retried attempt from leaking a stale `true` observed in an
// earlier iteration.
func (d *FirestoreDedup) CheckAndMark(ctx context.Context, keyCd string) (bool, error) {
	ref := d.client.Collection(d.collection).Doc(keyCd)
	var seenOnLastAttempt bool
	err := d.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// Reset on every attempt so the result reflects this iteration only.
		seenOnLastAttempt = false
		_, err := tx.Get(ref)
		if err == nil {
			seenOnLastAttempt = true
			return nil
		}
		if status.Code(err) != codes.NotFound {
			return err
		}
		return tx.Set(ref, map[string]interface{}{
			"expireAt":  time.Now().Add(d.ttl),
			"createdAt": time.Now(),
		})
	})
	if err != nil {
		return false, errs.Wrap("dedup.firestore", errs.KindExternal, err)
	}
	return seenOnLastAttempt, nil
}
