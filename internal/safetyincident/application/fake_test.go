package application_test

import (
	"context"
	"errors"
	"sync"

	"github.com/soneda-yuya/reearth-homework/internal/safetyincident/domain"
	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

// fakeMofaSource returns a fixed slice or a sticky error.
type fakeMofaSource struct {
	items map[domain.IngestionMode][]domain.MailItem
	err   error
}

func (f *fakeMofaSource) Fetch(_ context.Context, mode domain.IngestionMode) ([]domain.MailItem, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.items[mode], nil
}

// fakeLocationExtractor returns a canned (location, confidence) per key_cd.
// failOn maps key_cd → error to simulate per-item failures.
type fakeLocationExtractor struct {
	results map[string]domain.ExtractResult
	failOn  map[string]error
	mu      sync.Mutex
	calls   int
}

func (f *fakeLocationExtractor) Extract(_ context.Context, item domain.MailItem) (domain.ExtractResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if err, ok := f.failOn[item.KeyCd]; ok {
		return domain.ExtractResult{}, err
	}
	if r, ok := f.results[item.KeyCd]; ok {
		return r, nil
	}
	return domain.ExtractResult{Location: "default", Confidence: 0.8}, nil
}

// fakeGeocoder returns a canned point. failOn lets us simulate Geocode errors
// (after the chain has exhausted its fallbacks).
type fakeGeocoder struct {
	point  domain.Point
	source domain.GeocodeSource
	failOn map[string]error // keyed on country_cd
}

func (f *fakeGeocoder) Geocode(_ context.Context, _, countryCd string) (domain.GeocodeResult, error) {
	if err, ok := f.failOn[countryCd]; ok {
		return domain.GeocodeResult{}, err
	}
	return domain.GeocodeResult{Point: f.point, Source: f.source}, nil
}

// fakeRepository tracks Exists / Upsert calls, supports pre-seeding existing
// items, and lets a test inject Upsert errors per key_cd.
type fakeRepository struct {
	mu        sync.Mutex
	existing  map[string]bool // key_cd → exists
	saved     map[string]domain.SafetyIncident
	upsertErr map[string]error // key_cd → error to return
	existsErr error
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		existing:  map[string]bool{},
		saved:     map[string]domain.SafetyIncident{},
		upsertErr: map[string]error{},
	}
}

func (f *fakeRepository) Exists(_ context.Context, keyCd string) (bool, error) {
	if f.existsErr != nil {
		return false, f.existsErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.existing[keyCd], nil
}

func (f *fakeRepository) Upsert(_ context.Context, incident domain.SafetyIncident) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.upsertErr[incident.KeyCd]; ok {
		return err
	}
	f.saved[incident.KeyCd] = incident
	f.existing[incident.KeyCd] = true
	return nil
}

// fakeEventPublisher records every publish and supports per-key_cd errors.
type fakeEventPublisher struct {
	mu        sync.Mutex
	published []domain.NewArrivalEvent
	failOn    map[string]error
}

func (f *fakeEventPublisher) PublishNewArrival(_ context.Context, ev domain.NewArrivalEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.failOn[ev.KeyCd]; ok {
		return err
	}
	f.published = append(f.published, ev)
	return nil
}

// helper: a transient error class for fakes.
func transient(msg string) error {
	return errs.Wrap("fake", errs.KindExternal, errors.New(msg))
}
