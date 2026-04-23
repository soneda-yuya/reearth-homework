package application_test

import (
	"context"
	"testing"
	"time"

	"github.com/soneda-yuya/reearth-homework/internal/safetyincident/application"
	"github.com/soneda-yuya/reearth-homework/internal/safetyincident/domain"
	"github.com/soneda-yuya/reearth-homework/internal/shared/clock"
	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

func sampleItems(keyCds ...string) []domain.MailItem {
	out := make([]domain.MailItem, 0, len(keyCds))
	for _, k := range keyCds {
		out = append(out, domain.MailItem{
			KeyCd:     k,
			Title:     "title-" + k,
			LeaveDate: time.Date(2026, 4, 23, 10, 0, 0, 0, time.UTC),
			CountryCd: "JP",
			InfoType:  "info",
		})
	}
	return out
}

func defaultGeocoder() *fakeGeocoder {
	return &fakeGeocoder{
		point:  domain.Point{Lat: 35.6, Lng: 139.7},
		source: domain.GeocodeSourceMapbox,
	}
}

func newUseCase(
	t *testing.T,
	source domain.MofaSource,
	extractor domain.LocationExtractor,
	geocoder domain.Geocoder,
	repo domain.Repository,
	publisher domain.EventPublisher,
) *application.IngestUseCase {
	t.Helper()
	return application.NewIngestUseCase(
		source, extractor, geocoder, repo, publisher,
		application.Deps{
			Clock:       clock.Fixed(time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)),
			Concurrency: 2, // small concurrency keeps test ordering predictable
		},
	)
}

// Scenario 1: incremental run, all items new → all processed + all published.
func TestExecute_Incremental_AllNew(t *testing.T) {
	t.Parallel()
	source := &fakeMofaSource{items: map[domain.IngestionMode][]domain.MailItem{
		domain.IngestionModeIncremental: sampleItems("k-1", "k-2", "k-3"),
	}}
	extractor := &fakeLocationExtractor{results: map[string]domain.ExtractResult{}}
	geocoder := defaultGeocoder()
	repo := newFakeRepository()
	pub := &fakeEventPublisher{failOn: map[string]error{}}

	uc := newUseCase(t, source, extractor, geocoder, repo, pub)
	res, err := uc.Execute(context.Background(), application.IngestInput{Mode: domain.IngestionModeIncremental})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Fetched != 3 || res.Processed != 3 || res.Skipped != 0 || res.Published != 3 {
		t.Errorf("counters = %+v, want fetched=3 processed=3 skipped=0 published=3", res)
	}
	if len(repo.saved) != 3 {
		t.Errorf("repo saved = %d, want 3", len(repo.saved))
	}
	if len(pub.published) != 3 {
		t.Errorf("publisher recorded = %d, want 3", len(pub.published))
	}
}

// Scenario 2: incremental run with everything already in CMS → all skipped.
func TestExecute_Incremental_AllSkipped(t *testing.T) {
	t.Parallel()
	source := &fakeMofaSource{items: map[domain.IngestionMode][]domain.MailItem{
		domain.IngestionModeIncremental: sampleItems("k-1", "k-2"),
	}}
	repo := newFakeRepository()
	repo.existing["k-1"] = true
	repo.existing["k-2"] = true
	extractor := &fakeLocationExtractor{}
	pub := &fakeEventPublisher{}

	uc := newUseCase(t, source, extractor, defaultGeocoder(), repo, pub)
	res, err := uc.Execute(context.Background(), application.IngestInput{Mode: domain.IngestionModeIncremental})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Skipped != 2 || res.Processed != 0 || res.Published != 0 {
		t.Errorf("counters = %+v, want skipped=2 processed=0 published=0", res)
	}
	if extractor.calls != 0 {
		t.Errorf("extractor should be short-circuited, got %d calls", extractor.calls)
	}
}

// Scenario 3: initial mode → uses the initial slice and creates everything.
func TestExecute_InitialMode_UsesInitialPayload(t *testing.T) {
	t.Parallel()
	source := &fakeMofaSource{items: map[domain.IngestionMode][]domain.MailItem{
		domain.IngestionModeIncremental: nil,
		domain.IngestionModeInitial:     sampleItems("backfill-1", "backfill-2"),
	}}
	uc := newUseCase(t, source, &fakeLocationExtractor{}, defaultGeocoder(), newFakeRepository(), &fakeEventPublisher{})

	res, err := uc.Execute(context.Background(), application.IngestInput{Mode: domain.IngestionModeInitial})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Fetched != 2 || res.Processed != 2 {
		t.Errorf("counters = %+v, want fetched=2 processed=2", res)
	}
}

// Scenario 4: Mapbox failed → ChainGeocoder fell back to centroid. From the
// use case's perspective, Geocoder returned a result with Source=centroid;
// we just need to make sure the source flows through unchanged.
func TestExecute_GeocoderReturnsCentroidFallback(t *testing.T) {
	t.Parallel()
	source := &fakeMofaSource{items: map[domain.IngestionMode][]domain.MailItem{
		domain.IngestionModeIncremental: sampleItems("k-fb"),
	}}
	geocoder := &fakeGeocoder{
		point:  domain.Point{Lat: 36, Lng: 138}, // a JP centroid
		source: domain.GeocodeSourceCountryCentroid,
	}
	repo := newFakeRepository()
	uc := newUseCase(t, source, &fakeLocationExtractor{}, geocoder, repo, &fakeEventPublisher{})

	res, err := uc.Execute(context.Background(), application.IngestInput{Mode: domain.IngestionModeIncremental})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Processed != 1 {
		t.Errorf("processed = %d, want 1", res.Processed)
	}
	saved := repo.saved["k-fb"]
	if saved.GeocodeSource != domain.GeocodeSourceCountryCentroid {
		t.Errorf("saved.GeocodeSource = %s, want country_centroid", saved.GeocodeSource)
	}
}

// Scenario 5: per-item failures — one item fails at extract, one at upsert.
// Surviving items still process; the failed counters are populated.
func TestExecute_PartialFailures_SkipAndContinue(t *testing.T) {
	t.Parallel()
	source := &fakeMofaSource{items: map[domain.IngestionMode][]domain.MailItem{
		domain.IngestionModeIncremental: sampleItems("ok-1", "fail-extract", "fail-upsert", "ok-2"),
	}}
	extractor := &fakeLocationExtractor{
		failOn: map[string]error{"fail-extract": transient("LLM timeout")},
	}
	repo := newFakeRepository()
	repo.upsertErr["fail-upsert"] = transient("CMS 503")
	uc := newUseCase(t, source, extractor, defaultGeocoder(), repo, &fakeEventPublisher{})

	res, err := uc.Execute(context.Background(), application.IngestInput{Mode: domain.IngestionModeIncremental})
	if err != nil {
		t.Fatalf("unexpected error (skip-and-continue must not propagate): %v", err)
	}
	if res.Processed != 2 || res.Failed[application.PhaseExtract] != 1 || res.Failed[application.PhaseUpsert] != 1 {
		t.Errorf("counters = %+v, want processed=2 failed[extract]=1 failed[upsert]=1", res)
	}
	if _, ok := repo.saved["ok-1"]; !ok {
		t.Error("ok-1 should have been saved")
	}
	if _, ok := repo.saved["fail-extract"]; ok {
		t.Error("fail-extract should NOT have been saved")
	}
}

// Scenario 6: publish failure does not roll back the upsert. Item counts as
// processed, publish is recorded as failed (Warn-level metric).
func TestExecute_PublishFailureDoesNotRollback(t *testing.T) {
	t.Parallel()
	source := &fakeMofaSource{items: map[domain.IngestionMode][]domain.MailItem{
		domain.IngestionModeIncremental: sampleItems("k-pub-fail"),
	}}
	pub := &fakeEventPublisher{failOn: map[string]error{"k-pub-fail": transient("pubsub 500")}}
	repo := newFakeRepository()
	uc := newUseCase(t, source, &fakeLocationExtractor{}, defaultGeocoder(), repo, pub)

	res, err := uc.Execute(context.Background(), application.IngestInput{Mode: domain.IngestionModeIncremental})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Processed != 1 || res.Published != 0 || res.Failed[application.PhasePublish] != 1 {
		t.Errorf("counters = %+v, want processed=1 published=0 failed[publish]=1", res)
	}
	if _, ok := repo.saved["k-pub-fail"]; !ok {
		t.Error("upsert should still have happened despite publish failure")
	}
}

// Bonus: a fatal MOFA fetch returns an error from Execute (not exit 0). This
// is the only path that does NOT use skip-and-continue.
func TestExecute_FetchFailureBubblesUp(t *testing.T) {
	t.Parallel()
	source := &fakeMofaSource{err: transient("MOFA 503")}
	uc := newUseCase(t, source, &fakeLocationExtractor{}, defaultGeocoder(), newFakeRepository(), &fakeEventPublisher{})

	_, err := uc.Execute(context.Background(), application.IngestInput{Mode: domain.IngestionModeIncremental})
	if err == nil {
		t.Fatal("expected fetch error to bubble up")
	}
	if !errs.IsKind(err, errs.KindExternal) {
		t.Errorf("kind = %s, want KindExternal", errs.KindOf(err))
	}
}
