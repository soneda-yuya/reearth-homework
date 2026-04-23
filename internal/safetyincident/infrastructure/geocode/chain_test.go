package geocode_test

import (
	"context"
	"errors"
	"testing"

	"github.com/soneda-yuya/reearth-homework/internal/safetyincident/domain"
	"github.com/soneda-yuya/reearth-homework/internal/safetyincident/infrastructure/geocode"
	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

type stubMapboxLookup struct {
	point domain.Point
	ok    bool
	err   error
}

func (s *stubMapboxLookup) Lookup(_ context.Context, _, _ string) (domain.Point, bool, error) {
	return s.point, s.ok, s.err
}

type stubCentroid struct {
	point domain.Point
	err   error
}

func (s *stubCentroid) Lookup(_ string) (domain.Point, error) {
	return s.point, s.err
}

func TestChain_MapboxHit_ReturnsMapboxSource(t *testing.T) {
	t.Parallel()
	mapbox := &stubMapboxLookup{point: domain.Point{Lat: 35.6, Lng: 139.7}, ok: true}
	centroid := &stubCentroid{point: domain.Point{Lat: 36, Lng: 138}}
	chain := geocode.NewChainGeocoder(mapbox, centroid, nil)

	got, err := chain.Geocode(context.Background(), "Tokyo", "JP")
	if err != nil {
		t.Fatalf("Geocode: %v", err)
	}
	if got.Source != domain.GeocodeSourceMapbox {
		t.Errorf("Source = %s, want mapbox", got.Source)
	}
	if got.Point != mapbox.point {
		t.Errorf("Point = %v, want %v", got.Point, mapbox.point)
	}
}

func TestChain_MapboxMiss_FallsBackToCentroid(t *testing.T) {
	t.Parallel()
	mapbox := &stubMapboxLookup{ok: false}
	centroid := &stubCentroid{point: domain.Point{Lat: 36.2, Lng: 138.2}}
	chain := geocode.NewChainGeocoder(mapbox, centroid, nil)

	got, err := chain.Geocode(context.Background(), "Atlantis", "JP")
	if err != nil {
		t.Fatalf("Geocode: %v", err)
	}
	if got.Source != domain.GeocodeSourceCountryCentroid {
		t.Errorf("Source = %s, want country_centroid", got.Source)
	}
	if got.Point != centroid.point {
		t.Errorf("Point = %v, want %v", got.Point, centroid.point)
	}
}

func TestChain_MapboxError_FallsBackToCentroid(t *testing.T) {
	t.Parallel()
	mapbox := &stubMapboxLookup{err: errors.New("HTTP 500")}
	centroid := &stubCentroid{point: domain.Point{Lat: 36.2, Lng: 138.2}}
	chain := geocode.NewChainGeocoder(mapbox, centroid, nil)

	got, err := chain.Geocode(context.Background(), "Tokyo", "JP")
	if err != nil {
		t.Fatalf("Geocode: %v (mapbox failure should not propagate)", err)
	}
	if got.Source != domain.GeocodeSourceCountryCentroid {
		t.Errorf("Source = %s, want country_centroid", got.Source)
	}
}

func TestChain_EmptyLocation_SkipsMapboxAndUsesCentroid(t *testing.T) {
	t.Parallel()
	mapbox := &stubMapboxLookup{point: domain.Point{Lat: 0, Lng: 0}, ok: true} // would be returned if called
	centroid := &stubCentroid{point: domain.Point{Lat: 36.2, Lng: 138.2}}
	chain := geocode.NewChainGeocoder(mapbox, centroid, nil)

	got, err := chain.Geocode(context.Background(), "", "JP")
	if err != nil {
		t.Fatalf("Geocode: %v", err)
	}
	if got.Source != domain.GeocodeSourceCountryCentroid {
		t.Errorf("Source = %s, want country_centroid (mapbox should be skipped)", got.Source)
	}
}

func TestChain_CentroidUnknown_ReturnsKindNotFound(t *testing.T) {
	t.Parallel()
	mapbox := &stubMapboxLookup{ok: false}
	centroid := &stubCentroid{
		err: errs.Wrap("centroid", errs.KindNotFound, errors.New("unknown country")),
	}
	chain := geocode.NewChainGeocoder(mapbox, centroid, nil)

	_, err := chain.Geocode(context.Background(), "x", "ZZ")
	if err == nil {
		t.Fatal("expected error for unknown country")
	}
	if !errs.IsKind(err, errs.KindNotFound) {
		t.Errorf("kind = %s, want KindNotFound", errs.KindOf(err))
	}
}
