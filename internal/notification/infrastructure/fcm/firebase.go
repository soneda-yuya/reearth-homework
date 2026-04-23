// Package fcm wraps Firebase Cloud Messaging for the notifier BC. The
// Firebase Admin SDK is concurrency-safe and owns its own HTTP connections.
package fcm

import (
	"context"

	"firebase.google.com/go/v4/messaging"

	"github.com/soneda-yuya/reearth-homework/internal/notification/domain"
	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

// FirebaseFCM implements domain.FCMClient via firebase.google.com/go/v4.
type FirebaseFCM struct {
	client *messaging.Client
}

// New wires the adapter to a Firebase Messaging client (normally obtained
// from firebasex.Client.Messaging).
func New(client *messaging.Client) *FirebaseFCM {
	return &FirebaseFCM{client: client}
}

// SendMulticast delivers one notification to every token the user owns.
// Per-token errors are classified into Invalid (permanently dead) vs
// Transient (quota / timeout etc.) so the use case can remove the dead
// ones from Firestore in the same request.
func (f *FirebaseFCM) SendMulticast(ctx context.Context, msg domain.FCMMessage) (domain.BatchResult, error) {
	mc := &messaging.MulticastMessage{
		Tokens: msg.Tokens,
		Notification: &messaging.Notification{
			Title: msg.Title,
			Body:  msg.Body,
		},
		Data: msg.Data,
	}
	resp, err := f.client.SendEachForMulticast(ctx, mc)
	if err != nil {
		// This is a whole-call failure (auth / network / serialisation).
		// The use case logs it and records all tokens as failed.
		return domain.BatchResult{}, errs.Wrap("fcm.send_multicast", errs.KindExternal, err)
	}

	out := domain.BatchResult{
		SuccessCount: resp.SuccessCount,
		FailureCount: resp.FailureCount,
	}
	for i, r := range resp.Responses {
		if r.Error == nil {
			continue
		}
		token := msg.Tokens[i]
		if isPermanentTokenError(r.Error) {
			out.Invalid = append(out.Invalid, token)
		} else {
			out.Transient = append(out.Transient, token)
		}
	}
	return out, nil
}

// isPermanentTokenError decides whether to remove the token from Firestore.
// Firebase Admin SDK exposes concrete helpers for the two cases we care
// about: the user uninstalled the app, or FCM considers the token malformed.
func isPermanentTokenError(err error) bool {
	if err == nil {
		return false
	}
	return messaging.IsUnregistered(err) ||
		messaging.IsInvalidArgument(err) ||
		messaging.IsSenderIDMismatch(err)
}
