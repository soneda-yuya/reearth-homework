package application

import (
	"context"
	"log/slog"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"golang.org/x/sync/errgroup"

	"github.com/soneda-yuya/reearth-homework/internal/safetyincident/domain"
	"github.com/soneda-yuya/reearth-homework/internal/shared/clock"
	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

// Limiter is the subset of platform/ratelimit.Limiter that IngestUseCase
// uses. Declared here so tests can inject a no-op limiter without pulling
// in the real package.
type Limiter interface {
	Wait(ctx context.Context) error
}

// IngestUseCase orchestrates the per-Run pipeline for one MOFA mode. Failure
// strategy is skip-and-continue (U-ING design Q7 [A]); the only error this
// returns is a fatal MOFA fetch failure.
type IngestUseCase struct {
	source    domain.MofaSource
	extractor domain.LocationExtractor
	geocoder  domain.Geocoder
	repo      domain.Repository
	publisher domain.EventPublisher
	clock     clock.Clock
	logger    *slog.Logger
	tracer    trace.Tracer
	meter     metric.Meter

	llmLimiter     Limiter
	geocodeLimiter Limiter
	concurrency    int

	// counters are pre-created so per-item processing stays allocation-free.
	fetchedCounter   metric.Int64Counter
	skippedCounter   metric.Int64Counter
	processedCounter metric.Int64Counter
	failedCounter    metric.Int64Counter
	publishedCounter metric.Int64Counter
	fallbackCounter  metric.Int64Counter
}

// IngestInput selects which MOFA endpoint Source.Fetch will hit. The mode
// flows through to a span attribute so traces can be filtered by run kind.
type IngestInput struct {
	Mode domain.IngestionMode
}

// Deps groups the optional dependencies. NewIngestUseCase falls back to
// no-op observability and a sensible concurrency default when these are
// zero, so tests can pass an empty Deps and still get sound behaviour.
type Deps struct {
	Logger         *slog.Logger
	Tracer         trace.Tracer
	Meter          metric.Meter
	Clock          clock.Clock
	LLMLimiter     Limiter
	GeocodeLimiter Limiter
	Concurrency    int
}

// noopLimiter is the fallback used when the caller passes no real limiter.
// Wait always returns immediately, which matches "no rate limit applied".
type noopLimiter struct{}

func (noopLimiter) Wait(_ context.Context) error { return nil }

// NewIngestUseCase wires the use case. nil dependencies are replaced with
// safe defaults so tests don't have to construct an entire observability
// stack just to exercise behaviour.
func NewIngestUseCase(
	source domain.MofaSource,
	extractor domain.LocationExtractor,
	geocoder domain.Geocoder,
	repo domain.Repository,
	publisher domain.EventPublisher,
	deps Deps,
) *IngestUseCase {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	if deps.Tracer == nil {
		deps.Tracer = tracenoop.NewTracerProvider().Tracer("ingestion")
	}
	if deps.Meter == nil {
		deps.Meter = metricnoop.NewMeterProvider().Meter("ingestion")
	}
	if deps.Clock == nil {
		deps.Clock = clock.System()
	}
	if deps.LLMLimiter == nil {
		deps.LLMLimiter = noopLimiter{}
	}
	if deps.GeocodeLimiter == nil {
		deps.GeocodeLimiter = noopLimiter{}
	}
	if deps.Concurrency <= 0 {
		deps.Concurrency = 5
	}

	fetched, _ := deps.Meter.Int64Counter("app.ingestion.run.fetched")
	skipped, _ := deps.Meter.Int64Counter("app.ingestion.run.skipped")
	processed, _ := deps.Meter.Int64Counter("app.ingestion.run.processed")
	failed, _ := deps.Meter.Int64Counter("app.ingestion.run.failed")
	published, _ := deps.Meter.Int64Counter("app.ingestion.run.published")
	fallback, _ := deps.Meter.Int64Counter("app.ingestion.geocode.fallback")

	return &IngestUseCase{
		source:           source,
		extractor:        extractor,
		geocoder:         geocoder,
		repo:             repo,
		publisher:        publisher,
		clock:            deps.Clock,
		logger:           deps.Logger,
		tracer:           deps.Tracer,
		meter:            deps.Meter,
		llmLimiter:       deps.LLMLimiter,
		geocodeLimiter:   deps.GeocodeLimiter,
		concurrency:      deps.Concurrency,
		fetchedCounter:   fetched,
		skippedCounter:   skipped,
		processedCounter: processed,
		failedCounter:    failed,
		publishedCounter: published,
		fallbackCounter:  fallback,
	}
}

// Execute fetches one MOFA payload and processes every item concurrently
// (bounded by Concurrency). Per-item failures are skip-and-continue; the
// only fatal path is a fetch failure (which Cloud Scheduler will retry
// 5 minutes later — no need for in-process retry).
func (u *IngestUseCase) Execute(ctx context.Context, in IngestInput) (IngestResult, error) {
	ctx, span := u.tracer.Start(ctx, "ingestion.Run",
		trace.WithAttributes(attribute.String("mode", string(in.Mode))))
	defer span.End()

	items, err := u.fetch(ctx, in.Mode)
	if err != nil {
		return IngestResult{}, err
	}

	result := newResult()
	result.Fetched = len(items)
	if u.fetchedCounter != nil {
		u.fetchedCounter.Add(ctx, int64(len(items)),
			metric.WithAttributes(attribute.String("mode", string(in.Mode))))
	}

	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(u.concurrency)

	for _, item := range items {
		item := item
		g.Go(func() error {
			outcome := u.processItem(gctx, item)
			mu.Lock()
			defer mu.Unlock()
			result.Skipped += outcome.skipped
			result.Processed += outcome.processed
			result.Published += outcome.published
			if outcome.failedPhase != "" {
				result.Failed[outcome.failedPhase]++
			}
			if outcome.publishFailed {
				result.Failed[PhasePublish]++
			}
			return nil // never propagate; skip-and-continue
		})
	}
	_ = g.Wait()

	return result, nil
}

func (u *IngestUseCase) fetch(ctx context.Context, mode domain.IngestionMode) ([]domain.MailItem, error) {
	ctx, span := u.tracer.Start(ctx, "ingestion.Fetch")
	defer span.End()
	items, err := u.source.Fetch(ctx, mode)
	if err != nil {
		u.logger.ErrorContext(ctx, "mofa fetch failed",
			"app.ingestion.phase", "fetch",
			"err", err,
		)
		if u.failedCounter != nil {
			u.failedCounter.Add(ctx, 1,
				metric.WithAttributes(attribute.String("phase", string(PhaseFetch))))
		}
		return nil, errs.Wrap("ingestion.fetch", errs.KindOf(err), err)
	}
	return items, nil
}

// itemOutcome is the per-item summary the goroutine sends back to the
// aggregator. publishFailed is tracked separately because an item can be
// processed (CMS upsert succeeded) yet still record a publish failure —
// we want both signals visible in the result.
type itemOutcome struct {
	skipped       int // 0 or 1
	processed     int // 0 or 1
	published     int // 0 or 1
	failedPhase   Phase
	publishFailed bool
}

func (u *IngestUseCase) processItem(ctx context.Context, item domain.MailItem) itemOutcome {
	ctx, span := u.tracer.Start(ctx, "ingestion.ProcessItem",
		trace.WithAttributes(attribute.String("key_cd", item.KeyCd)))
	defer span.End()

	// Fail fast on malformed items — MOFA occasionally emits rows with
	// missing required fields (empty title, missing country_cd). We record
	// the failure under `phase=validate` so they surface in metrics instead
	// of producing less actionable failures downstream (e.g. Mapbox 404 on
	// an empty country code).
	if err := item.Validate(); err != nil {
		u.recordItemFailure(ctx, item, PhaseValidate, err)
		return itemOutcome{failedPhase: PhaseValidate}
	}

	exists, err := u.repo.Exists(ctx, item.KeyCd)
	if err != nil {
		u.recordItemFailure(ctx, item, PhaseLookup, err)
		return itemOutcome{failedPhase: PhaseLookup}
	}
	if exists {
		if u.skippedCounter != nil {
			u.skippedCounter.Add(ctx, 1)
		}
		return itemOutcome{skipped: 1}
	}

	if err := u.llmLimiter.Wait(ctx); err != nil {
		u.recordItemFailure(ctx, item, PhaseExtract, err)
		return itemOutcome{failedPhase: PhaseExtract}
	}
	extract, err := u.extractor.Extract(ctx, item)
	if err != nil {
		u.recordItemFailure(ctx, item, PhaseExtract, err)
		return itemOutcome{failedPhase: PhaseExtract}
	}

	if err := u.geocodeLimiter.Wait(ctx); err != nil {
		u.recordItemFailure(ctx, item, PhaseGeocode, err)
		return itemOutcome{failedPhase: PhaseGeocode}
	}
	geocode, err := u.geocoder.Geocode(ctx, extract.Location, item.CountryCd)
	if err != nil {
		u.recordItemFailure(ctx, item, PhaseGeocode, err)
		return itemOutcome{failedPhase: PhaseGeocode}
	}
	// Count fallback only — the metric name promises "fallback", so Mapbox
	// successes should not inflate it. Operators alert on fallback ratio as
	// a signal that geocoding quality is degraded.
	if geocode.Source == domain.GeocodeSourceCountryCentroid && u.fallbackCounter != nil {
		u.fallbackCounter.Add(ctx, 1,
			metric.WithAttributes(attribute.String("source", geocode.Source.String())))
	}

	incident := domain.Build(item, extract, geocode, u.clock.Now())
	if err := u.repo.Upsert(ctx, incident); err != nil {
		u.recordItemFailure(ctx, item, PhaseUpsert, err)
		return itemOutcome{failedPhase: PhaseUpsert}
	}
	if u.processedCounter != nil {
		u.processedCounter.Add(ctx, 1)
	}

	publishedCount := 0
	publishFailed := false
	pubErr := u.publisher.PublishNewArrival(ctx, domain.NewArrivalEvent{
		KeyCd:     incident.KeyCd,
		CountryCd: incident.CountryCd,
		InfoType:  incident.InfoType,
		Geometry:  incident.Geometry,
		LeaveDate: incident.LeaveDate,
	})
	if pubErr != nil {
		// CMS already has the item — log a warning and increment the failed
		// counter, but do NOT mark the item failed: the upsert succeeded so
		// the operator can rebroadcast manually if needed.
		u.logger.WarnContext(ctx, "publish failed (CMS upsert succeeded)",
			"app.ingestion.phase", string(PhasePublish),
			"key_cd", incident.KeyCd,
			"err", pubErr,
		)
		if u.failedCounter != nil {
			u.failedCounter.Add(ctx, 1,
				metric.WithAttributes(attribute.String("phase", string(PhasePublish))))
		}
		publishFailed = true
	} else {
		publishedCount = 1
		if u.publishedCounter != nil {
			u.publishedCounter.Add(ctx, 1)
		}
	}

	return itemOutcome{processed: 1, published: publishedCount, publishFailed: publishFailed}
}

func (u *IngestUseCase) recordItemFailure(ctx context.Context, item domain.MailItem, phase Phase, err error) {
	u.logger.ErrorContext(ctx, "item processing failed",
		"app.ingestion.phase", string(phase),
		"key_cd", item.KeyCd,
		"err", err,
	)
	if u.failedCounter != nil {
		u.failedCounter.Add(ctx, 1,
			metric.WithAttributes(attribute.String("phase", string(phase))))
	}
}
