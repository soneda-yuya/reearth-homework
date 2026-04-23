package domain

// FCMMessage is the per-user payload sent to Firebase Cloud Messaging via
// SendEachForMulticast. Tokens is the concrete device list for one user;
// Data / Notification are the same across all recipients of one event.
type FCMMessage struct {
	Tokens []string
	Title  string
	Body   string
	Data   map[string]string // key_cd / country_cd / info_type
}

// BatchResult is the domain-shaped outcome of one SendEachForMulticast call.
// The adapter classifies per-token errors into Invalid (permanently dead —
// registration-token-not-registered / invalid-argument) vs Transient
// (quota / timeout). The use case removes Invalid tokens from Firestore in
// the same request; Transient tokens are left alone for Pub/Sub retry.
type BatchResult struct {
	SuccessCount int
	FailureCount int
	Invalid      []string
	Transient    []string
}
