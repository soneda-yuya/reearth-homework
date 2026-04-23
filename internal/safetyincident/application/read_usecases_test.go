package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/application"
	"github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/domain"
	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
)

func newFixtureIncidents() []domain.SafetyIncident {
	mk := func(keyCd, country string, lat, lng float64, src domain.GeocodeSource) domain.SafetyIncident {
		return domain.SafetyIncident{
			MailItem: domain.MailItem{
				KeyCd:       keyCd,
				InfoType:    "spot_info",
				Title:       "title-" + keyCd,
				CountryCd:   country,
				CountryName: "Country-" + country,
				LeaveDate:   time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
			},
			Geometry:      domain.Point{Lat: lat, Lng: lng},
			GeocodeSource: src,
		}
	}
	return []domain.SafetyIncident{
		mk("K1", "JP", 35.0, 139.0, domain.GeocodeSourceMapbox),
		mk("K2", "JP", 34.7, 135.5, domain.GeocodeSourceMapbox),
		mk("K3", "US", 40.0, -74.0, domain.GeocodeSourceMapbox),
		mk("K4", "US", 39.0, -77.0, domain.GeocodeSourceCountryCentroid),
	}
}

func TestListUseCase_PassesFilterAndCursor(t *testing.T) {
	t.Parallel()
	reader := &fakeReader{items: newFixtureIncidents(), nextCursor: "next-token"}
	uc := application.NewListUseCase(reader)

	items, next, err := uc.Execute(context.Background(), domain.ListFilter{CountryCd: "JP", Cursor: "prev"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if next != "next-token" {
		t.Errorf("nextCursor = %q; want next-token", next)
	}
	if len(items) != 4 {
		t.Errorf("len(items) = %d; want 4", len(items))
	}
	if reader.lastList.CountryCd != "JP" || reader.lastList.Cursor != "prev" {
		t.Errorf("filter not forwarded: %+v", reader.lastList)
	}
}

func TestListUseCase_ReaderError(t *testing.T) {
	t.Parallel()
	reader := &fakeReader{listErr: errs.Wrap("reader", errs.KindExternal, errors.New("boom"))}
	uc := application.NewListUseCase(reader)
	if _, _, err := uc.Execute(context.Background(), domain.ListFilter{}); !errs.IsKind(err, errs.KindExternal) {
		t.Errorf("err = %v; want KindExternal", err)
	}
}

func TestGetUseCase(t *testing.T) {
	t.Parallel()
	reader := &fakeReader{items: newFixtureIncidents()}
	uc := application.NewGetUseCase(reader)
	got, err := uc.Execute(context.Background(), "K1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got.KeyCd != "K1" {
		t.Errorf("KeyCd = %q; want K1", got.KeyCd)
	}
	if _, err := uc.Execute(context.Background(), "MISSING"); !errs.IsKind(err, errs.KindNotFound) {
		t.Errorf("missing err = %v; want KindNotFound", err)
	}
}

func TestSearchUseCase_PassesFilter(t *testing.T) {
	t.Parallel()
	reader := &fakeReader{items: newFixtureIncidents()}
	uc := application.NewSearchUseCase(reader)
	_, _, err := uc.Execute(context.Background(), domain.SearchFilter{Query: "earthquake"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if reader.lastSearch.Query != "earthquake" {
		t.Errorf("Search query not forwarded: %+v", reader.lastSearch)
	}
}

func TestNearbyUseCase(t *testing.T) {
	t.Parallel()
	reader := &fakeReader{items: newFixtureIncidents()}
	uc := application.NewNearbyUseCase(reader)
	center := domain.Point{Lat: 35, Lng: 139}
	items, err := uc.Execute(context.Background(), center, 500, 10)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(items) != 4 {
		t.Errorf("len = %d", len(items))
	}
	if reader.lastCenter != center || reader.lastRadius != 500 || reader.lastLimit != 10 {
		t.Errorf("args not forwarded: center=%v radius=%v limit=%v", reader.lastCenter, reader.lastRadius, reader.lastLimit)
	}
}

func TestGeoJSONUseCase_EncodesFeatureCollection(t *testing.T) {
	t.Parallel()
	reader := &fakeReader{items: newFixtureIncidents()}
	uc := application.NewGeoJSONUseCase(reader)
	fc, err := uc.Execute(context.Background(), domain.ListFilter{Limit: 100})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if fc.Type != "FeatureCollection" {
		t.Errorf("Type = %q", fc.Type)
	}
	if len(fc.Features) != 4 {
		t.Fatalf("features = %d", len(fc.Features))
	}
	first := fc.Features[0]
	if first.Geometry.Type != "Point" {
		t.Errorf("geometry type = %q", first.Geometry.Type)
	}
	// GeoJSON orders coordinates [lng, lat].
	if first.Geometry.Coordinates[0] != 139.0 || first.Geometry.Coordinates[1] != 35.0 {
		t.Errorf("coordinates order wrong: %v", first.Geometry.Coordinates)
	}
	if first.Properties.KeyCd != "K1" {
		t.Errorf("props.KeyCd = %q", first.Properties.KeyCd)
	}
}

func TestGeoJSONUseCase_ReaderError(t *testing.T) {
	t.Parallel()
	reader := &fakeReader{listErr: errs.Wrap("reader", errs.KindExternal, errors.New("boom"))}
	uc := application.NewGeoJSONUseCase(reader)
	if _, err := uc.Execute(context.Background(), domain.ListFilter{}); !errs.IsKind(err, errs.KindExternal) {
		t.Errorf("err = %v; want KindExternal", err)
	}
}
