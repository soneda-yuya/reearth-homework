package application_test

import (
	"context"
	"errors"
	"sync"

	"github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/domain"
	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
)

// fakeReader is a SafetyIncidentReader stub used by the BFF read-side use
// cases. It records the last filter each method saw so tests can assert the
// parameters the use case passed through.
type fakeReader struct {
	mu sync.Mutex

	items      []domain.SafetyIncident
	nextCursor string
	listErr    error
	getErr     error
	searchErr  error
	nearbyErr  error
	lastList   domain.ListFilter
	lastSearch domain.SearchFilter
	lastCenter domain.Point
	lastRadius float64
	lastLimit  int
}

func (f *fakeReader) List(_ context.Context, filter domain.ListFilter) ([]domain.SafetyIncident, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastList = filter
	if f.listErr != nil {
		return nil, "", f.listErr
	}
	return f.items, f.nextCursor, nil
}

func (f *fakeReader) Get(_ context.Context, keyCd string) (*domain.SafetyIncident, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	for _, it := range f.items {
		if it.KeyCd == keyCd {
			cp := it
			return &cp, nil
		}
	}
	return nil, errs.Wrap("fakeReader.Get", errs.KindNotFound, errors.New("not found"))
}

func (f *fakeReader) Search(_ context.Context, filter domain.SearchFilter) ([]domain.SafetyIncident, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastSearch = filter
	if f.searchErr != nil {
		return nil, "", f.searchErr
	}
	return f.items, f.nextCursor, nil
}

func (f *fakeReader) ListNearby(_ context.Context, center domain.Point, radiusKm float64, limit int) ([]domain.SafetyIncident, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastCenter = center
	f.lastRadius = radiusKm
	f.lastLimit = limit
	if f.nearbyErr != nil {
		return nil, f.nearbyErr
	}
	return f.items, nil
}
