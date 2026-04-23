package application

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"golang.org/x/sync/errgroup"

	"github.com/soneda-yuya/overseas-safety-map/internal/notification/domain"
	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
)

// DeliverNotificationUseCase orchestrates one Pub/Sub push delivery:
// dedup → resolve subscribers → parallel FCM send → clean up invalid tokens.
// Per-user FCM failures are absorbed (logged, never bubble); only
// dedup/resolve transport errors propagate as errors (→ HTTP 500).
type DeliverNotificationUseCase struct {
	dedup  domain.Dedup
	users  domain.UserRepository
	fcm    domain.FCMClient
	logger *slog.Logger
	tracer trace.Tracer
	meter  metric.Meter

	concurrency int

	receivedCounter         metric.Int64Counter
	dedupedCounter          metric.Int64Counter
	recipientsHistogram     metric.Int64Histogram
	fcmSentCounter          metric.Int64Counter
	tokenInvalidatedCounter metric.Int64Counter
	durationHistogram       metric.Int64Histogram
}

// DeliverInput carries the incoming event through the use case. Raw envelope
// parsing lives in the interfaces layer.
type DeliverInput struct {
	Event domain.NewArrivalEvent
}

// Deps groups optional dependencies. Nil fields fall back to no-op OTel /
// slog.Default / concurrency=5 so tests can pass zero values.
type Deps struct {
	Logger      *slog.Logger
	Tracer      trace.Tracer
	Meter       metric.Meter
	Concurrency int
}

// NewDeliverNotificationUseCase wires the use case with sensible defaults.
func NewDeliverNotificationUseCase(
	dedup domain.Dedup,
	users domain.UserRepository,
	fcm domain.FCMClient,
	deps Deps,
) *DeliverNotificationUseCase {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	if deps.Tracer == nil {
		deps.Tracer = tracenoop.NewTracerProvider().Tracer("notifier")
	}
	if deps.Meter == nil {
		deps.Meter = metricnoop.NewMeterProvider().Meter("notifier")
	}
	if deps.Concurrency <= 0 {
		deps.Concurrency = 5
	}

	received, _ := deps.Meter.Int64Counter("app.notifier.received")
	deduped, _ := deps.Meter.Int64Counter("app.notifier.deduped")
	recipients, _ := deps.Meter.Int64Histogram("app.notifier.recipients")
	fcmSent, _ := deps.Meter.Int64Counter("app.notifier.fcm.sent")
	tokenInvalidated, _ := deps.Meter.Int64Counter("app.notifier.fcm.token_invalidated")
	duration, _ := deps.Meter.Int64Histogram("app.notifier.duration")

	return &DeliverNotificationUseCase{
		dedup:                   dedup,
		users:                   users,
		fcm:                     fcm,
		logger:                  deps.Logger,
		tracer:                  deps.Tracer,
		meter:                   deps.Meter,
		concurrency:             deps.Concurrency,
		receivedCounter:         received,
		dedupedCounter:          deduped,
		recipientsHistogram:     recipients,
		fcmSentCounter:          fcmSent,
		tokenInvalidatedCounter: tokenInvalidated,
		durationHistogram:       duration,
	}
}

// Execute runs the full delivery pipeline. Returns an error only when the
// push should NOT be ACKed (dedup / users lookup transient failure). FCM
// partial failures are recorded but the overall call still returns nil so
// the handler ACKs with 200.
func (u *DeliverNotificationUseCase) Execute(ctx context.Context, in DeliverInput) (DeliverResult, error) {
	ctx, span := u.tracer.Start(ctx, "notifier.Deliver",
		trace.WithAttributes(
			attribute.String("key_cd", in.Event.KeyCd),
			attribute.String("country_cd", in.Event.CountryCd),
			attribute.String("info_type", in.Event.InfoType),
		))
	defer span.End()

	// Record per-request duration on every exit path (including dedup /
	// no-subscribers early returns and transient-error propagation). The
	// outcome attribute lets dashboards separate happy-path p95 from the
	// dedup/no-sub shortcuts.
	start := time.Now()
	outcome := OutcomeDelivered // default; overridden below on early exits
	defer func() {
		if u.durationHistogram == nil {
			return
		}
		u.durationHistogram.Record(ctx, time.Since(start).Milliseconds(),
			metric.WithAttributes(attribute.String("outcome", outcome.String())))
	}()

	if u.receivedCounter != nil {
		u.receivedCounter.Add(ctx, 1, metric.WithAttributes(
			attribute.String("country_cd", in.Event.CountryCd),
			attribute.String("info_type", in.Event.InfoType),
		))
	}

	// --- Dedup --------------------------------------------------------
	alreadySeen, err := u.checkDedup(ctx, in.Event.KeyCd)
	if err != nil {
		return DeliverResult{}, err
	}
	if alreadySeen {
		u.logger.InfoContext(ctx, "dedup hit",
			"app.notifier.phase", "dedup",
			"key_cd", in.Event.KeyCd,
		)
		if u.dedupedCounter != nil {
			u.dedupedCounter.Add(ctx, 1)
		}
		outcome = OutcomeDeduped
		return DeliverResult{Outcome: OutcomeDeduped}, nil
	}

	// --- Resolve subscribers -----------------------------------------
	subscribers, err := u.resolveSubscribers(ctx, in.Event)
	if err != nil {
		return DeliverResult{}, err
	}
	if len(subscribers) == 0 {
		u.logger.InfoContext(ctx, "no subscribers",
			"app.notifier.phase", "resolve",
			"key_cd", in.Event.KeyCd,
		)
		outcome = OutcomeNoSubscribers
		return DeliverResult{Outcome: OutcomeNoSubscribers}, nil
	}
	if u.recipientsHistogram != nil {
		u.recipientsHistogram.Record(ctx, int64(len(subscribers)))
	}

	// --- Parallel FCM send + cleanup ---------------------------------
	result := u.deliverToAll(ctx, in.Event, subscribers)
	result.Outcome = OutcomeDelivered
	result.RecipientsCount = len(subscribers)

	u.logger.InfoContext(ctx, "delivered",
		"app.notifier.phase", "done",
		"key_cd", in.Event.KeyCd,
		"recipients", result.RecipientsCount,
		"fcm_success", result.FCMSuccessTokens,
		"fcm_failed", result.FCMFailedTokens,
		"invalidated", result.InvalidTokensRemoved,
	)
	return result, nil
}

func (u *DeliverNotificationUseCase) checkDedup(ctx context.Context, keyCd string) (bool, error) {
	ctx, span := u.tracer.Start(ctx, "dedup.CheckAndMark")
	defer span.End()
	alreadySeen, err := u.dedup.CheckAndMark(ctx, keyCd)
	if err != nil {
		return false, errs.Wrap("notifier.dedup", errs.KindOf(err), err)
	}
	return alreadySeen, nil
}

func (u *DeliverNotificationUseCase) resolveSubscribers(ctx context.Context, ev domain.NewArrivalEvent) ([]domain.UserProfile, error) {
	ctx, span := u.tracer.Start(ctx, "users.Resolve")
	defer span.End()
	subs, err := u.users.FindSubscribers(ctx, ev.CountryCd, ev.InfoType)
	if err != nil {
		return nil, errs.Wrap("notifier.resolve", errs.KindOf(err), err)
	}
	return subs, nil
}

func (u *DeliverNotificationUseCase) deliverToAll(ctx context.Context, ev domain.NewArrivalEvent, subs []domain.UserProfile) DeliverResult {
	var (
		mu     sync.Mutex
		result DeliverResult
	)

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(u.concurrency)
	for _, s := range subs {
		s := s
		g.Go(func() error {
			outcome := u.deliverToUser(gctx, ev, s)
			mu.Lock()
			defer mu.Unlock()
			result.FCMSuccessTokens += outcome.success
			result.FCMFailedTokens += outcome.failed
			result.InvalidTokensRemoved += outcome.invalidated
			return nil // never propagate: per-user failure is a warning
		})
	}
	_ = g.Wait()
	return result
}

type userOutcome struct {
	success, failed, invalidated int
}

func (u *DeliverNotificationUseCase) deliverToUser(ctx context.Context, ev domain.NewArrivalEvent, sub domain.UserProfile) userOutcome {
	ctx, span := u.tracer.Start(ctx, "fcm.SendMulticast",
		trace.WithAttributes(attribute.String("uid", sub.UID)))
	defer span.End()

	msg := domain.FCMMessage{
		Tokens: sub.FCMTokens,
		Title:  ev.Title,
		Body:   buildBody(ev),
		Data: map[string]string{
			"key_cd":     ev.KeyCd,
			"country_cd": ev.CountryCd,
			"info_type":  ev.InfoType,
		},
	}
	batch, err := u.fcm.SendMulticast(ctx, msg)
	if err != nil {
		// Whole-call failure (all tokens failed together): log & continue.
		u.logger.WarnContext(ctx, "fcm send failed for user",
			"app.notifier.phase", "send",
			"uid", sub.UID,
			"err", err,
		)
		if u.fcmSentCounter != nil {
			u.fcmSentCounter.Add(ctx, int64(len(sub.FCMTokens)),
				metric.WithAttributes(attribute.String("status", "failure")))
		}
		return userOutcome{failed: len(sub.FCMTokens)}
	}

	if u.fcmSentCounter != nil {
		if batch.SuccessCount > 0 {
			u.fcmSentCounter.Add(ctx, int64(batch.SuccessCount),
				metric.WithAttributes(attribute.String("status", "success")))
		}
		if batch.FailureCount > 0 {
			u.fcmSentCounter.Add(ctx, int64(batch.FailureCount),
				metric.WithAttributes(attribute.String("status", "failure")))
		}
	}

	invalidated := 0
	if len(batch.Invalid) > 0 {
		if err := u.users.RemoveInvalidTokens(ctx, sub.UID, batch.Invalid); err != nil {
			u.logger.WarnContext(ctx, "remove invalid tokens failed",
				"app.notifier.phase", "cleanup",
				"uid", sub.UID,
				"err", err,
			)
		} else {
			invalidated = len(batch.Invalid)
			if u.tokenInvalidatedCounter != nil {
				u.tokenInvalidatedCounter.Add(ctx, int64(invalidated))
			}
		}
	}

	return userOutcome{
		success:     batch.SuccessCount,
		failed:      batch.FailureCount,
		invalidated: invalidated,
	}
}

// buildBody crafts the notification body from the event. Kept tiny — the
// authoritative SafetyIncident is in reearth-cms and the Flutter app fetches
// the full record when the user taps the notification.
func buildBody(ev domain.NewArrivalEvent) string {
	if ev.Lead != "" {
		return ev.Lead
	}
	// Fallback: 位置 + 種別を簡潔に表示。
	return fmt.Sprintf("%s (%s) に関する新しい情報が届きました", ev.CountryCd, ev.InfoType)
}
