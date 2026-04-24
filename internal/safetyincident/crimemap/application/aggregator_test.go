package application_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/crimemap/application"
	crimemap "github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/crimemap/domain"
	safetyincident "github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/domain"
	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
)

type stubReader struct {
	mu     sync.Mutex
	items  []safetyincident.SafetyIncident
	err    error
	called int
}

func (s *stubReader) List(_ context.Context, _ safetyincident.ListFilter) ([]safetyincident.SafetyIncident, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.called++
	if s.err != nil {
		return nil, "", s.err
	}
	return s.items, "", nil
}
func (s *stubReader) Get(context.Context, string) (*safetyincident.SafetyIncident, error) {
	return nil, errors.New("unused")
}
func (s *stubReader) Search(context.Context, safetyincident.SearchFilter) ([]safetyincident.SafetyIncident, string, error) {
	return nil, "", errors.New("unused")
}
func (s *stubReader) ListNearby(context.Context, safetyincident.Point, float64, int) ([]safetyincident.SafetyIncident, error) {
	return nil, errors.New("unused")
}

func fixture() []safetyincident.SafetyIncident {
	mk := func(key, country string, lat, lng float64, src safetyincident.GeocodeSource) safetyincident.SafetyIncident {
		return safetyincident.SafetyIncident{
			MailItem: safetyincident.MailItem{
				KeyCd: key, CountryCd: country, CountryName: "Country-" + country,
				LeaveDate: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
			},
			Geometry:      safetyincident.Point{Lat: lat, Lng: lng},
			GeocodeSource: src,
		}
	}
	return []safetyincident.SafetyIncident{
		mk("K1", "JP", 35, 139, safetyincident.GeocodeSourceMapbox),
		mk("K2", "JP", 34, 135, safetyincident.GeocodeSourceMapbox),
		mk("K3", "JP", 33, 131, safetyincident.GeocodeSourceMapbox),
		mk("K4", "US", 40, -74, safetyincident.GeocodeSourceMapbox),
		mk("K5", "US", 39, -77, safetyincident.GeocodeSourceCountryCentroid),
	}
}

func TestChoropleth_Aggregation(t *testing.T) {
	t.Parallel()
	reader := &stubReader{items: fixture()}
	agg := application.NewAggregator(reader)
	res, err := agg.Choropleth(context.Background(), crimemap.CrimeMapFilter{})
	if err != nil {
		t.Fatalf("Choropleth: %v", err)
	}
	if res.Total != 5 {
		t.Errorf("Total = %d; want 5", res.Total)
	}
	if len(res.Items) != 2 {
		t.Fatalf("len(Items) = %d; want 2 (JP + US)", len(res.Items))
	}
	counts := map[string]int{}
	colors := map[string]string{}
	for _, it := range res.Items {
		counts[it.CountryCd] = it.Count
		colors[it.CountryCd] = it.Color
	}
	if counts["JP"] != 3 || counts["US"] != 2 {
		t.Errorf("counts = %v; want JP=3 US=2", counts)
	}
	if colors["JP"] == "" || colors["US"] == "" {
		t.Errorf("color missing: %v", colors)
	}
	// JP has max count (3), so it must paint the darkest.
	if colors["JP"] != "#a50f15" {
		t.Errorf("JP color = %q; want darkest #a50f15 (max bucket)", colors["JP"])
	}
}

func TestChoropleth_SkipsEmptyCountry(t *testing.T) {
	t.Parallel()
	items := []safetyincident.SafetyIncident{
		{
			MailItem: safetyincident.MailItem{
				KeyCd: "K1", CountryCd: "JP", CountryName: "日本",
				LeaveDate: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
			},
			Geometry:      safetyincident.Point{Lat: 35, Lng: 139},
			GeocodeSource: safetyincident.GeocodeSourceMapbox,
		},
		// cd present, name missing — must be omitted from the choropleth.
		{
			MailItem: safetyincident.MailItem{
				KeyCd: "K2", CountryCd: "US", CountryName: "",
				LeaveDate: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
			},
			Geometry:      safetyincident.Point{Lat: 40, Lng: -74},
			GeocodeSource: safetyincident.GeocodeSourceMapbox,
		},
		// Both missing — must be omitted too.
		{
			MailItem: safetyincident.MailItem{
				KeyCd: "K3", CountryCd: "", CountryName: "",
				LeaveDate: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
			},
			Geometry:      safetyincident.Point{Lat: 0, Lng: 0},
			GeocodeSource: safetyincident.GeocodeSourceCountryCentroid,
		},
	}
	agg := application.NewAggregator(&stubReader{items: items})
	res, err := agg.Choropleth(context.Background(), crimemap.CrimeMapFilter{})
	if err != nil {
		t.Fatalf("Choropleth: %v", err)
	}
	// Total still reflects the corpus size so the UI "全 N 件" legend is
	// honest about how many items were considered.
	if res.Total != 3 {
		t.Errorf("Total = %d; want 3 (corpus count is unaffected by the filter)", res.Total)
	}
	if len(res.Items) != 1 {
		t.Fatalf("len(Items) = %d; want 1 (only JP has a country name)", len(res.Items))
	}
	if res.Items[0].CountryCd != "JP" || res.Items[0].CountryName != "日本" {
		t.Errorf("Items[0] = %+v; want JP/日本", res.Items[0])
	}
	if res.Items[0].Count != 1 {
		t.Errorf("JP count = %d; want 1", res.Items[0].Count)
	}
}

func TestChoropleth_EmptyCorpus(t *testing.T) {
	t.Parallel()
	agg := application.NewAggregator(&stubReader{})
	res, err := agg.Choropleth(context.Background(), crimemap.CrimeMapFilter{})
	if err != nil {
		t.Fatalf("Choropleth: %v", err)
	}
	if res.Items == nil {
		t.Error("Items must be non-nil empty slice, not nil")
	}
	if res.Total != 0 {
		t.Errorf("Total = %d", res.Total)
	}
}

func TestChoropleth_ReaderError(t *testing.T) {
	t.Parallel()
	agg := application.NewAggregator(&stubReader{err: errs.Wrap("reader", errs.KindExternal, errors.New("boom"))})
	if _, err := agg.Choropleth(context.Background(), crimemap.CrimeMapFilter{}); !errs.IsKind(err, errs.KindExternal) {
		t.Errorf("err = %v", err)
	}
}

func TestHeatmap_ExcludesCentroidFallback(t *testing.T) {
	t.Parallel()
	reader := &stubReader{items: fixture()}
	agg := application.NewAggregator(reader)
	res, err := agg.Heatmap(context.Background(), crimemap.CrimeMapFilter{})
	if err != nil {
		t.Fatalf("Heatmap: %v", err)
	}
	if len(res.Points) != 4 {
		t.Errorf("len(Points) = %d; want 4 (5 incidents - 1 centroid)", len(res.Points))
	}
	if res.ExcludedFallback != 1 {
		t.Errorf("ExcludedFallback = %d; want 1", res.ExcludedFallback)
	}
	for _, p := range res.Points {
		if p.Weight != 1.0 {
			t.Errorf("Weight = %v; want 1.0 (MVP default)", p.Weight)
		}
	}
}

func TestHeatmap_ReaderError(t *testing.T) {
	t.Parallel()
	agg := application.NewAggregator(&stubReader{err: errs.Wrap("reader", errs.KindExternal, errors.New("boom"))})
	if _, err := agg.Heatmap(context.Background(), crimemap.CrimeMapFilter{}); !errs.IsKind(err, errs.KindExternal) {
		t.Errorf("err = %v", err)
	}
}
