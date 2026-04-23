package geocode_test

import (
	"context"
	"errors"
	"testing"

	"github.com/soneda-yuya/reearth-homework/internal/platform/mapboxx"
	"github.com/soneda-yuya/reearth-homework/internal/safetyincident/infrastructure/geocode"
)

type stubMapboxClient struct {
	result mapboxx.GeocodeResult
	err    error
}

func (s *stubMapboxClient) Geocode(_ context.Context, _, _ string) (mapboxx.GeocodeResult, error) {
	return s.result, s.err
}

func TestMapboxGeocoder_HighRelevance_ReturnsPoint(t *testing.T) {
	t.Parallel()
	stub := &stubMapboxClient{
		result: mapboxx.GeocodeResult{Lat: 35.69, Lng: 139.69, Relevance: 0.92},
	}
	g := geocode.NewMapboxGeocoder(stub, 0.5)
	point, ok, err := g.Lookup(context.Background(), "Tokyo", "JP")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !ok {
		t.Fatal("ok should be true for high relevance")
	}
	if point.Lat != 35.69 || point.Lng != 139.69 {
		t.Errorf("point = %v", point)
	}
}

func TestMapboxGeocoder_LowRelevance_ReturnsNotOK(t *testing.T) {
	t.Parallel()
	stub := &stubMapboxClient{
		result: mapboxx.GeocodeResult{Lat: 1, Lng: 2, Relevance: 0.3},
	}
	g := geocode.NewMapboxGeocoder(stub, 0.5)
	_, ok, err := g.Lookup(context.Background(), "vague", "FR")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if ok {
		t.Error("ok should be false for relevance below threshold")
	}
}

func TestMapboxGeocoder_EmptyLocation_SkipsLookup(t *testing.T) {
	t.Parallel()
	stub := &stubMapboxClient{
		result: mapboxx.GeocodeResult{Lat: 99, Lng: 99, Relevance: 1.0},
	}
	g := geocode.NewMapboxGeocoder(stub, 0.5)
	_, ok, err := g.Lookup(context.Background(), "", "JP")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if ok {
		t.Error("empty location should always return ok=false")
	}
}

func TestMapboxGeocoder_TransportError_PropagatesUp(t *testing.T) {
	t.Parallel()
	stub := &stubMapboxClient{err: errors.New("HTTP 503")}
	g := geocode.NewMapboxGeocoder(stub, 0.5)
	_, _, err := g.Lookup(context.Background(), "Paris", "FR")
	if err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestMapboxGeocoder_NoFeatures_ReturnsNotOK(t *testing.T) {
	t.Parallel()
	stub := &stubMapboxClient{} // zero value = empty result
	g := geocode.NewMapboxGeocoder(stub, 0.5)
	_, ok, err := g.Lookup(context.Background(), "Atlantis", "JP")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if ok {
		t.Error("zero result should produce ok=false")
	}
}
