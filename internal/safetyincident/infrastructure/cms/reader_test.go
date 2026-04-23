package cms_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/soneda-yuya/overseas-safety-map/internal/platform/cmsx"
	"github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/domain"
	"github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/infrastructure/cms"
	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
)

type stubReaderClient struct {
	listRes   cmsx.ListItemsResult
	listErr   error
	searchRes cmsx.ListItemsResult
	searchErr error
	findRes   *cmsx.ItemDTO
	findErr   error

	lastListQuery   cmsx.ListItemsQuery
	lastSearchQuery cmsx.ListItemsQuery
}

func (s *stubReaderClient) ListItems(_ context.Context, _ string, q cmsx.ListItemsQuery) (cmsx.ListItemsResult, error) {
	s.lastListQuery = q
	return s.listRes, s.listErr
}

func (s *stubReaderClient) SearchItems(_ context.Context, _ string, q cmsx.ListItemsQuery) (cmsx.ListItemsResult, error) {
	s.lastSearchQuery = q
	return s.searchRes, s.searchErr
}

func (s *stubReaderClient) FindItemByFieldValue(_ context.Context, _, _, _ string) (*cmsx.ItemDTO, error) {
	return s.findRes, s.findErr
}

// itemFields builds the CMS-shaped fields map matching what toFields writes.
func itemFields(keyCd, country string, lat, lng float64, src string) map[string]any {
	return map[string]any{
		"key_cd":             keyCd,
		"info_type":          "spot_info",
		"title":              "title-" + keyCd,
		"country_cd":         country,
		"country_name":       "Country-" + country,
		"leave_date":         time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"ingested_at":        time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"updated_at":         time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"geometry":           map[string]any{"type": "Point", "coordinates": []any{lng, lat}},
		"geocode_source":     src,
		"extracted_location": "somewhere",
	}
}

func TestReaderList_DecodesItemsAndForwardsCursor(t *testing.T) {
	t.Parallel()
	client := &stubReaderClient{
		listRes: cmsx.ListItemsResult{
			Items: []cmsx.ItemDTO{
				{ID: "i-1", Fields: itemFields("K1", "JP", 35, 139, "mapbox")},
				{ID: "i-2", Fields: itemFields("K2", "US", 40, -74, "country_centroid")},
			},
			NextCursor: "next-token",
		},
	}
	reader := cms.NewReader(client, "m-1", "key_cd")

	items, next, err := reader.List(context.Background(), domain.ListFilter{
		CountryCd: "JP",
		InfoTypes: []string{"spot_info"},
		LeaveFrom: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		Limit:     50,
		Cursor:    "prev",
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("len(items) = %d", len(items))
	}
	if next != "next-token" {
		t.Errorf("next = %q", next)
	}
	if client.lastListQuery.Filters["country_cd"] != "JP" {
		t.Errorf("country_cd filter not forwarded: %+v", client.lastListQuery)
	}
	if client.lastListQuery.Cursor != "prev" {
		t.Errorf("cursor = %q", client.lastListQuery.Cursor)
	}
	if items[0].GeocodeSource != domain.GeocodeSourceMapbox {
		t.Errorf("GeocodeSource = %v; want mapbox", items[0].GeocodeSource)
	}
	if items[1].GeocodeSource != domain.GeocodeSourceCountryCentroid {
		t.Errorf("GeocodeSource[1] = %v; want country_centroid", items[1].GeocodeSource)
	}
}

func TestReaderList_ClientError(t *testing.T) {
	t.Parallel()
	client := &stubReaderClient{listErr: errs.Wrap("net", errs.KindExternal, errors.New("boom"))}
	reader := cms.NewReader(client, "m-1", "key_cd")
	if _, _, err := reader.List(context.Background(), domain.ListFilter{}); !errs.IsKind(err, errs.KindExternal) {
		t.Errorf("err = %v", err)
	}
}

func TestReaderGet_Hit(t *testing.T) {
	t.Parallel()
	client := &stubReaderClient{findRes: &cmsx.ItemDTO{ID: "i-1", Fields: itemFields("K1", "JP", 35, 139, "mapbox")}}
	reader := cms.NewReader(client, "m-1", "key_cd")
	got, err := reader.Get(context.Background(), "K1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.KeyCd != "K1" {
		t.Errorf("KeyCd = %q", got.KeyCd)
	}
}

func TestReaderGet_Miss(t *testing.T) {
	t.Parallel()
	client := &stubReaderClient{findRes: nil}
	reader := cms.NewReader(client, "m-1", "key_cd")
	_, err := reader.Get(context.Background(), "MISSING")
	if !errs.IsKind(err, errs.KindNotFound) {
		t.Errorf("err = %v; want KindNotFound", err)
	}
}

func TestReaderGet_EmptyKey(t *testing.T) {
	t.Parallel()
	reader := cms.NewReader(&stubReaderClient{}, "m-1", "key_cd")
	if _, err := reader.Get(context.Background(), ""); !errs.IsKind(err, errs.KindInvalidInput) {
		t.Errorf("err = %v; want KindInvalidInput", err)
	}
}

func TestReaderSearch_PassesKeyword(t *testing.T) {
	t.Parallel()
	client := &stubReaderClient{searchRes: cmsx.ListItemsResult{Items: []cmsx.ItemDTO{}}}
	reader := cms.NewReader(client, "m-1", "key_cd")
	if _, _, err := reader.Search(context.Background(), domain.SearchFilter{Query: "earthquake", CountryCd: "JP"}); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if client.lastSearchQuery.Keyword != "earthquake" {
		t.Errorf("keyword = %q", client.lastSearchQuery.Keyword)
	}
	if client.lastSearchQuery.Filters["country_cd"] != "JP" {
		t.Errorf("country filter = %v", client.lastSearchQuery.Filters)
	}
}

func TestReaderListNearby_FiltersByDistance(t *testing.T) {
	t.Parallel()
	// Tokyo (35.68, 139.76), Osaka (34.69, 135.5 — ~400km), Sapporo (43.06, 141.35 — ~830km)
	client := &stubReaderClient{
		listRes: cmsx.ListItemsResult{
			Items: []cmsx.ItemDTO{
				{ID: "t", Fields: itemFields("K_TOKYO", "JP", 35.68, 139.76, "mapbox")},
				{ID: "o", Fields: itemFields("K_OSAKA", "JP", 34.69, 135.5, "mapbox")},
				{ID: "s", Fields: itemFields("K_SAPPORO", "JP", 43.06, 141.35, "mapbox")},
			},
		},
	}
	reader := cms.NewReader(client, "m-1", "key_cd")
	center := domain.Point{Lat: 35.68, Lng: 139.76}
	items, err := reader.ListNearby(context.Background(), center, 500, 10)
	if err != nil {
		t.Fatalf("ListNearby: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("len = %d; want 2 (Tokyo + Osaka, Sapporo ~830km should be excluded)", len(items))
	}
	if items[0].KeyCd != "K_TOKYO" {
		t.Errorf("first = %q; want K_TOKYO (closest to center)", items[0].KeyCd)
	}
}

func TestReaderListNearby_InvalidArgs(t *testing.T) {
	t.Parallel()
	reader := cms.NewReader(&stubReaderClient{}, "m-1", "key_cd")
	if _, err := reader.ListNearby(context.Background(), domain.Point{}, 0, 10); !errs.IsKind(err, errs.KindInvalidInput) {
		t.Errorf("radius=0 err = %v; want KindInvalidInput", err)
	}
	if _, err := reader.ListNearby(context.Background(), domain.Point{}, 100, 0); !errs.IsKind(err, errs.KindInvalidInput) {
		t.Errorf("limit=0 err = %v; want KindInvalidInput", err)
	}
}

func TestFromFields_MissingRequired(t *testing.T) {
	t.Parallel()
	client := &stubReaderClient{
		listRes: cmsx.ListItemsResult{Items: []cmsx.ItemDTO{{ID: "i-1", Fields: map[string]any{"key_cd": "K1"}}}},
	}
	reader := cms.NewReader(client, "m-1", "key_cd")
	_, _, err := reader.List(context.Background(), domain.ListFilter{})
	if !errs.IsKind(err, errs.KindInternal) {
		t.Errorf("err = %v; want KindInternal (schema drift)", err)
	}
}
