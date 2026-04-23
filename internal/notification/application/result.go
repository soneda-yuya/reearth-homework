// Package application holds the notification use cases. It depends on
// [domain] ports only; Firestore / FCM concrete clients live in
// internal/notification/infrastructure.
package application

// DeliverOutcome is the categorical result of one Pub/Sub push handled by
// the use case. The HTTP handler maps each value to a specific status code
// per Design Q6 [A].
type DeliverOutcome int

const (
	// OutcomeDelivered means the event reached at least one subscriber
	// (even if some individual tokens failed).
	OutcomeDelivered DeliverOutcome = iota
	// OutcomeDeduped means the event was already processed recently; the
	// use case short-circuits without calling FCM.
	OutcomeDeduped
	// OutcomeNoSubscribers means no user asked to receive this event.
	OutcomeNoSubscribers
)

// String returns the wire-friendly name used in structured logs.
func (o DeliverOutcome) String() string {
	switch o {
	case OutcomeDelivered:
		return "delivered"
	case OutcomeDeduped:
		return "deduped"
	case OutcomeNoSubscribers:
		return "no_subscribers"
	default:
		return "unknown"
	}
}

// DeliverResult is the richer summary the use case emits. The composition
// root logs these counters on every request.
type DeliverResult struct {
	Outcome              DeliverOutcome
	RecipientsCount      int
	FCMSuccessTokens     int
	FCMFailedTokens      int
	InvalidTokensRemoved int
}
