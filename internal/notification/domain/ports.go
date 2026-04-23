package domain

import "context"

// Dedup is the U-NTF idempotency store. Implementations back it with
// Firestore + TTL (24h); a second message with the same KeyCd within the
// TTL window returns alreadySeen=true so the use case can short-circuit.
type Dedup interface {
	// CheckAndMark is atomic: if the keyCd is unseen, mark it and return
	// (false, nil); if it already exists, return (true, nil) without
	// touching the record. Transport / transaction errors return non-nil.
	CheckAndMark(ctx context.Context, keyCd string) (alreadySeen bool, err error)
}

// UserRepository reads subscribers for a given event and removes tokens
// that FCM has declared permanently dead. Writes to the `users` collection
// beyond token removal belong to the user BC (U-BFF) and are out of scope
// here.
type UserRepository interface {
	// FindSubscribers returns users that asked to be notified about the
	// given (countryCd, infoType). The implementation may do the info_type
	// filter in memory — Firestore only supports one array-contains per
	// query. Implementations must skip users whose fcm_tokens is empty.
	FindSubscribers(ctx context.Context, countryCd, infoType string) ([]UserProfile, error)

	// RemoveInvalidTokens ArrayRemoves the listed tokens from the user's
	// fcm_tokens field. Failure is non-fatal at the use case level and is
	// logged at WARN — the notification itself has already been delivered.
	RemoveInvalidTokens(ctx context.Context, uid string, tokens []string) error
}

// FCMClient is the outbound Firebase Cloud Messaging port. The concrete
// adapter wraps firebase.google.com/go/v4/messaging and classifies
// per-token errors into BatchResult.Invalid / .Transient.
type FCMClient interface {
	SendMulticast(ctx context.Context, msg FCMMessage) (BatchResult, error)
}

// EventDecoder parses a raw Pub/Sub push HTTP body into a domain event.
// A parse failure is KindInvalidInput — the handler maps that to HTTP 400
// and Pub/Sub routes the malformed message directly to the DLQ.
type EventDecoder interface {
	Decode(body []byte) (NewArrivalEvent, error)
}
