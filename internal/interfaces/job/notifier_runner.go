// Package job hosts the HTTP handler surface for jobs / workers that live
// behind an HTTP boundary (e.g. Cloud Run Service triggered by Pub/Sub push).
package job

import (
	"io"
	"log/slog"
	"net/http"

	"github.com/soneda-yuya/overseas-safety-map/internal/notification/application"
	"github.com/soneda-yuya/overseas-safety-map/internal/notification/domain"
	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
)

// maxPushBodyBytes caps how much the handler will read from one Pub/Sub
// push request. Google caps messages at 10 MiB including attributes; we
// keep a 1 MiB ceiling which is plenty for NewArrivalEvent payloads (a few
// KB in practice) and protects us against misconfigured invokers or a stray
// tester POSTing a giant body.
const maxPushBodyBytes = 1 << 20

// NotifierHandler bridges Pub/Sub Push delivery onto
// [application.DeliverNotificationUseCase]. HTTP status codes follow the
// Q6 [A] contract: 200 for ACK, 400 for malformed payloads (Pub/Sub routes
// them straight to the DLQ), 500 for transient failures (Pub/Sub retries
// with backoff, eventually DLQ).
type NotifierHandler struct {
	decoder domain.EventDecoder
	usecase *application.DeliverNotificationUseCase
	logger  *slog.Logger
}

// NewNotifierHandler wires the handler. A nil logger falls back to slog.Default.
func NewNotifierHandler(decoder domain.EventDecoder, uc *application.DeliverNotificationUseCase, logger *slog.Logger) *NotifierHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &NotifierHandler{decoder: decoder, usecase: uc, logger: logger}
}

// Push is the HTTP handler for the Pub/Sub push endpoint. Register at
// POST /pubsub/push.
func (h *NotifierHandler) Push(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx := r.Context()

	defer func() { _ = r.Body.Close() }()
	limited := http.MaxBytesReader(w, r.Body, maxPushBodyBytes)
	body, err := io.ReadAll(limited)
	if err != nil {
		// Transport-level read failure — let Pub/Sub retry. Oversize
		// bodies also land here (MaxBytesReader returns an error once the
		// limit is hit); we classify as 500 too because Pub/Sub will not
		// shrink the payload on its own.
		h.logger.ErrorContext(ctx, "read push body failed",
			"app.notifier.phase", "receive",
			"err", err,
		)
		http.Error(w, "read body", http.StatusInternalServerError)
		return
	}

	event, err := h.decoder.Decode(body)
	if err != nil {
		// Malformed payload → immediate DLQ route via 400.
		h.logger.ErrorContext(ctx, "malformed push payload",
			"app.notifier.phase", "receive",
			"err", err,
		)
		http.Error(w, "malformed payload", http.StatusBadRequest)
		return
	}

	result, err := h.usecase.Execute(ctx, application.DeliverInput{Event: event})
	if err != nil {
		// Transient error (dedup/users SDK failure) → 500, Pub/Sub retries.
		h.logger.ErrorContext(ctx, "deliver failed",
			"app.notifier.phase", "failed",
			"key_cd", event.KeyCd,
			"err", err,
			"kind", string(errs.KindOf(err)),
		)
		http.Error(w, "deliver failed", http.StatusInternalServerError)
		return
	}

	// Success / Deduped / NoSubscribers all map to 200 — see Q6 [A].
	h.logger.InfoContext(ctx, "notifier finished",
		"app.notifier.phase", "done",
		"outcome", result.Outcome.String(),
		"key_cd", event.KeyCd,
		"recipients", result.RecipientsCount,
		"fcm_success", result.FCMSuccessTokens,
		"fcm_failed", result.FCMFailedTokens,
		"invalidated", result.InvalidTokensRemoved,
	)
	w.WriteHeader(http.StatusOK)
}

// Health is a simple 200 OK handler for Cloud Run startup/liveness probes.
func (h *NotifierHandler) Health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
