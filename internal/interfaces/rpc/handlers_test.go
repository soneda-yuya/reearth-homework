package rpc_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"

	overseasmapv1 "github.com/soneda-yuya/overseas-safety-map/gen/go/v1"
	"github.com/soneda-yuya/overseas-safety-map/gen/go/v1/overseasmapv1connect"
	"github.com/soneda-yuya/overseas-safety-map/internal/interfaces/rpc"
	"github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/application"
	crimemapapp "github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/crimemap/application"
	"github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/domain"
	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
	userapp "github.com/soneda-yuya/overseas-safety-map/internal/user/application"
	userdom "github.com/soneda-yuya/overseas-safety-map/internal/user/domain"
)

// sampleReader serves the BFF handler tests: a canned, 2-item corpus.
// lastSearchFilter lets tests assert the SearchFilter the handler forwarded,
// which is how we guard against the query / cursor wiring bug that motivated
// proto's dedicated query field.
type sampleReader struct {
	mu               sync.Mutex
	items            []domain.SafetyIncident
	nextCursor       string
	lastSearchFilter domain.SearchFilter
}

func (r *sampleReader) List(context.Context, domain.ListFilter) ([]domain.SafetyIncident, string, error) {
	return r.items, r.nextCursor, nil
}
func (r *sampleReader) Get(_ context.Context, keyCd string) (*domain.SafetyIncident, error) {
	for _, it := range r.items {
		if it.KeyCd == keyCd {
			cp := it
			return &cp, nil
		}
	}
	return nil, errs.Wrap("sampleReader", errs.KindNotFound, errors.New("no such item"))
}
func (r *sampleReader) Search(_ context.Context, filter domain.SearchFilter) ([]domain.SafetyIncident, string, error) {
	r.mu.Lock()
	r.lastSearchFilter = filter
	r.mu.Unlock()
	return r.items, r.nextCursor, nil
}
func (r *sampleReader) ListNearby(context.Context, domain.Point, float64, int) ([]domain.SafetyIncident, error) {
	return r.items, nil
}

func newSampleItems() []domain.SafetyIncident {
	return []domain.SafetyIncident{
		{
			MailItem: domain.MailItem{
				KeyCd:       "K1",
				InfoType:    "spot_info",
				Title:       "Earthquake",
				CountryCd:   "JP",
				CountryName: "Japan",
				LeaveDate:   time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
			},
			Geometry:      domain.Point{Lat: 35, Lng: 139},
			GeocodeSource: domain.GeocodeSourceMapbox,
		},
		{
			MailItem: domain.MailItem{
				KeyCd:       "K2",
				InfoType:    "warning",
				Title:       "Protest",
				CountryCd:   "US",
				CountryName: "United States",
				LeaveDate:   time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
			},
			Geometry:      domain.Point{Lat: 40, Lng: -74},
			GeocodeSource: domain.GeocodeSourceMapbox,
		},
	}
}

func newSafetyIncidentTestServer(t *testing.T) (overseasmapv1connect.SafetyIncidentServiceClient, *sampleReader) {
	t.Helper()
	reader := &sampleReader{items: newSampleItems()}
	srv := rpc.NewSafetyIncidentServer(
		application.NewListUseCase(reader),
		application.NewGetUseCase(reader),
		application.NewSearchUseCase(reader),
		application.NewNearbyUseCase(reader),
		application.NewGeoJSONUseCase(reader),
	)
	mux := http.NewServeMux()
	path, handler := overseasmapv1connect.NewSafetyIncidentServiceHandler(srv, connect.WithInterceptors(
		rpc.NewErrorInterceptor("dev"),
		rpc.NewAuthInterceptor(&fakeVerifier{accept: map[string]string{"t": "uid"}}, slog.Default()),
	))
	mux.Handle(path, handler)
	h := httptest.NewServer(mux)
	t.Cleanup(h.Close)
	return overseasmapv1connect.NewSafetyIncidentServiceClient(h.Client(), h.URL), reader
}

func withAuth(req connect.AnyRequest) {
	req.Header().Set("Authorization", "Bearer t")
}

func TestSafetyIncident_ListRoundTrip(t *testing.T) {
	t.Parallel()
	client, _ := newSafetyIncidentTestServer(t)
	req := connect.NewRequest(&overseasmapv1.ListSafetyIncidentsRequest{
		Filter: &overseasmapv1.SafetyIncidentFilter{CountryCd: "JP"},
	})
	withAuth(req)
	res, err := client.ListSafetyIncidents(context.Background(), req)
	if err != nil {
		t.Fatalf("ListSafetyIncidents: %v", err)
	}
	if len(res.Msg.Items) != 2 {
		t.Errorf("items = %d", len(res.Msg.Items))
	}
	if res.Msg.Items[0].KeyCd != "K1" {
		t.Errorf("first key = %q", res.Msg.Items[0].KeyCd)
	}
}

func TestSafetyIncident_GetHitAndMiss(t *testing.T) {
	t.Parallel()
	client, _ := newSafetyIncidentTestServer(t)
	// Hit
	reqHit := connect.NewRequest(&overseasmapv1.GetSafetyIncidentRequest{KeyCd: "K1"})
	withAuth(reqHit)
	res, err := client.GetSafetyIncident(context.Background(), reqHit)
	if err != nil {
		t.Fatalf("Hit: %v", err)
	}
	if res.Msg.Item.CountryCd != "JP" {
		t.Errorf("country = %q", res.Msg.Item.CountryCd)
	}
	// Miss — KindNotFound must surface as CodeNotFound.
	reqMiss := connect.NewRequest(&overseasmapv1.GetSafetyIncidentRequest{KeyCd: "MISSING"})
	withAuth(reqMiss)
	_, err = client.GetSafetyIncident(context.Background(), reqMiss)
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("miss code = %v; want NotFound", connect.CodeOf(err))
	}
}

func TestSafetyIncident_GeoJSON(t *testing.T) {
	t.Parallel()
	client, _ := newSafetyIncidentTestServer(t)
	req := connect.NewRequest(&overseasmapv1.GetSafetyIncidentsAsGeoJSONRequest{
		Filter: &overseasmapv1.SafetyIncidentFilter{Limit: 100},
	})
	withAuth(req)
	res, err := client.GetSafetyIncidentsAsGeoJSON(context.Background(), req)
	if err != nil {
		t.Fatalf("GeoJSON: %v", err)
	}
	var fc struct {
		Type     string `json:"type"`
		Features []struct {
			Geometry struct {
				Coordinates [2]float64 `json:"coordinates"`
			} `json:"geometry"`
		} `json:"features"`
	}
	if err := json.Unmarshal(res.Msg.Geojson, &fc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if fc.Type != "FeatureCollection" {
		t.Errorf("type = %q", fc.Type)
	}
	if len(fc.Features) != 2 {
		t.Errorf("features = %d", len(fc.Features))
	}
	if fc.Features[0].Geometry.Coordinates[0] != 139 {
		t.Errorf("lng = %v (coordinates must be [lng, lat])", fc.Features[0].Geometry.Coordinates[0])
	}
}

func TestSafetyIncident_SearchRoundTrip(t *testing.T) {
	t.Parallel()
	client, _ := newSafetyIncidentTestServer(t)
	req := connect.NewRequest(&overseasmapv1.SearchSafetyIncidentsRequest{
		Filter: &overseasmapv1.SafetyIncidentFilter{CountryCd: "JP"},
	})
	withAuth(req)
	res, err := client.SearchSafetyIncidents(context.Background(), req)
	if err != nil {
		t.Fatalf("SearchSafetyIncidents: %v", err)
	}
	if len(res.Msg.Items) != 2 {
		t.Errorf("items = %d", len(res.Msg.Items))
	}
}

// TestSafetyIncident_SearchForwardsQuery guards the proto `query` field ↔
// domain.SearchFilter.Query wiring. The handler must forward Request.Query
// into the reader filter without losing it on the cursor field — the exact
// regression that motivated adding `string query = 2` to the proto.
func TestSafetyIncident_SearchForwardsQuery(t *testing.T) {
	t.Parallel()
	client, reader := newSafetyIncidentTestServer(t)
	req := connect.NewRequest(&overseasmapv1.SearchSafetyIncidentsRequest{
		Filter: &overseasmapv1.SafetyIncidentFilter{CountryCd: "JP", Cursor: "page-2"},
		Query:  "earthquake",
	})
	withAuth(req)
	if _, err := client.SearchSafetyIncidents(context.Background(), req); err != nil {
		t.Fatalf("SearchSafetyIncidents: %v", err)
	}
	reader.mu.Lock()
	got := reader.lastSearchFilter
	reader.mu.Unlock()
	if got.Query != "earthquake" {
		t.Errorf("reader saw Query = %q; want earthquake", got.Query)
	}
	if got.Cursor != "page-2" {
		t.Errorf("reader saw Cursor = %q; want page-2 (cursor must not be stolen by query)", got.Cursor)
	}
	if got.CountryCd != "JP" {
		t.Errorf("reader saw CountryCd = %q", got.CountryCd)
	}
}

func TestSafetyIncident_ListNearby(t *testing.T) {
	t.Parallel()
	client, _ := newSafetyIncidentTestServer(t)
	req := connect.NewRequest(&overseasmapv1.ListNearbyRequest{
		Center:   &overseasmapv1.Point{Lat: 35, Lng: 139},
		RadiusKm: 1000,
		Limit:    10,
	})
	withAuth(req)
	res, err := client.ListNearby(context.Background(), req)
	if err != nil {
		t.Fatalf("ListNearby: %v", err)
	}
	if len(res.Msg.Items) != 2 {
		t.Errorf("items = %d", len(res.Msg.Items))
	}
}

// --- CrimeMap ---

func newCrimeMapTestServer(t *testing.T) overseasmapv1connect.CrimeMapServiceClient {
	t.Helper()
	reader := &sampleReader{items: newSampleItems()}
	srv := rpc.NewCrimeMapServer(crimemapapp.NewAggregator(reader))
	mux := http.NewServeMux()
	path, handler := overseasmapv1connect.NewCrimeMapServiceHandler(srv, connect.WithInterceptors(
		rpc.NewErrorInterceptor("dev"),
		rpc.NewAuthInterceptor(&fakeVerifier{accept: map[string]string{"t": "uid"}}, slog.Default()),
	))
	mux.Handle(path, handler)
	h := httptest.NewServer(mux)
	t.Cleanup(h.Close)
	return overseasmapv1connect.NewCrimeMapServiceClient(h.Client(), h.URL)
}

func TestCrimeMap_Choropleth(t *testing.T) {
	t.Parallel()
	client := newCrimeMapTestServer(t)
	req := connect.NewRequest(&overseasmapv1.GetChoroplethRequest{})
	withAuth(req)
	res, err := client.GetChoropleth(context.Background(), req)
	if err != nil {
		t.Fatalf("GetChoropleth: %v", err)
	}
	if len(res.Msg.Items) != 2 {
		t.Errorf("items = %d; want 2 (JP, US)", len(res.Msg.Items))
	}
	if res.Msg.Total != 2 {
		t.Errorf("total = %d", res.Msg.Total)
	}
}

func TestCrimeMap_Heatmap(t *testing.T) {
	t.Parallel()
	client := newCrimeMapTestServer(t)
	req := connect.NewRequest(&overseasmapv1.GetHeatmapRequest{})
	withAuth(req)
	res, err := client.GetHeatmap(context.Background(), req)
	if err != nil {
		t.Fatalf("GetHeatmap: %v", err)
	}
	if len(res.Msg.Points) != 2 {
		t.Errorf("points = %d", len(res.Msg.Points))
	}
}

// --- UserProfile ---

type profileStore struct {
	data map[string]userdom.UserProfile
}

func newProfileStore() *profileStore {
	return &profileStore{data: make(map[string]userdom.UserProfile)}
}

func (s *profileStore) Get(_ context.Context, uid string) (*userdom.UserProfile, error) {
	p, ok := s.data[uid]
	if !ok {
		return nil, errs.Wrap("store.get", errs.KindNotFound, errors.New("no such uid"))
	}
	cp := p
	return &cp, nil
}
func (s *profileStore) CreateIfMissing(_ context.Context, p userdom.UserProfile) error {
	if _, ok := s.data[p.UID]; !ok {
		s.data[p.UID] = p
	}
	return nil
}
func (s *profileStore) ToggleFavoriteCountry(_ context.Context, uid, cc string) error {
	p := s.data[uid]
	for i, v := range p.FavoriteCountryCds {
		if v == cc {
			p.FavoriteCountryCds = append(p.FavoriteCountryCds[:i], p.FavoriteCountryCds[i+1:]...)
			s.data[uid] = p
			return nil
		}
	}
	p.FavoriteCountryCds = append(p.FavoriteCountryCds, cc)
	s.data[uid] = p
	return nil
}
func (s *profileStore) UpdateNotificationPreference(_ context.Context, uid string, pref userdom.NotificationPreference) error {
	p := s.data[uid]
	p.NotificationPreference = pref
	s.data[uid] = p
	return nil
}
func (s *profileStore) RegisterFcmToken(_ context.Context, uid, tok string) error {
	p := s.data[uid]
	for _, t := range p.FCMTokens {
		if t == tok {
			return nil
		}
	}
	p.FCMTokens = append(p.FCMTokens, tok)
	s.data[uid] = p
	return nil
}

func newUserProfileTestServer(t *testing.T) (overseasmapv1connect.UserProfileServiceClient, *profileStore) {
	t.Helper()
	store := newProfileStore()
	srv := rpc.NewUserProfileServer(
		userapp.NewGetProfileUseCase(store),
		userapp.NewToggleFavoriteCountryUseCase(store),
		userapp.NewUpdateNotificationPreferenceUseCase(store),
		userapp.NewRegisterFcmTokenUseCase(store),
	)
	mux := http.NewServeMux()
	path, handler := overseasmapv1connect.NewUserProfileServiceHandler(srv, connect.WithInterceptors(
		rpc.NewErrorInterceptor("dev"),
		rpc.NewAuthInterceptor(&fakeVerifier{accept: map[string]string{"t": "uid-7"}}, slog.Default()),
	))
	mux.Handle(path, handler)
	h := httptest.NewServer(mux)
	t.Cleanup(h.Close)
	return overseasmapv1connect.NewUserProfileServiceClient(h.Client(), h.URL), store
}

func TestUserProfile_GetLazyCreates(t *testing.T) {
	t.Parallel()
	client, store := newUserProfileTestServer(t)
	req := connect.NewRequest(&overseasmapv1.GetProfileRequest{})
	withAuth(req)
	res, err := client.GetProfile(context.Background(), req)
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if res.Msg.Profile.Uid != "uid-7" {
		t.Errorf("uid = %q", res.Msg.Profile.Uid)
	}
	if _, ok := store.data["uid-7"]; !ok {
		t.Error("lazy create did not persist")
	}
}

func TestUserProfile_ToggleFavorite(t *testing.T) {
	t.Parallel()
	client, store := newUserProfileTestServer(t)
	req := connect.NewRequest(&overseasmapv1.ToggleFavoriteCountryRequest{CountryCd: "JP"})
	withAuth(req)
	if _, err := client.ToggleFavoriteCountry(context.Background(), req); err != nil {
		t.Fatalf("Toggle: %v", err)
	}
	if got := store.data["uid-7"].FavoriteCountryCds; len(got) != 1 || got[0] != "JP" {
		t.Errorf("store = %v", got)
	}
}

func TestUserProfile_UpdatePreference(t *testing.T) {
	t.Parallel()
	client, store := newUserProfileTestServer(t)
	req := connect.NewRequest(&overseasmapv1.UpdateNotificationPreferenceRequest{
		Preference: &overseasmapv1.NotificationPreference{
			Enabled:          true,
			TargetCountryCds: []string{"JP"},
			InfoTypes:        []string{"spot_info"},
		},
	})
	withAuth(req)
	if _, err := client.UpdateNotificationPreference(context.Background(), req); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got := store.data["uid-7"].NotificationPreference
	if !got.Enabled || len(got.TargetCountryCds) != 1 {
		t.Errorf("prefs = %+v", got)
	}
}

func TestUserProfile_RegisterFcmToken(t *testing.T) {
	t.Parallel()
	client, store := newUserProfileTestServer(t)
	req := connect.NewRequest(&overseasmapv1.RegisterFcmTokenRequest{Token: "tok-A"})
	withAuth(req)
	if _, err := client.RegisterFcmToken(context.Background(), req); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if got := store.data["uid-7"].FCMTokens; len(got) != 1 {
		t.Errorf("tokens = %v", got)
	}
}

func TestUserProfile_NoAuthIsUnauthenticated(t *testing.T) {
	t.Parallel()
	client, _ := newUserProfileTestServer(t)
	_, err := client.GetProfile(context.Background(), connect.NewRequest(&overseasmapv1.GetProfileRequest{}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Errorf("code = %v", connect.CodeOf(err))
	}
}
