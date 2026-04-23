package cms_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/soneda-yuya/reearth-homework/internal/platform/cmsx"
	"github.com/soneda-yuya/reearth-homework/internal/safetyincident/domain"
	"github.com/soneda-yuya/reearth-homework/internal/safetyincident/infrastructure/cms"
	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

type stubItemClient struct {
	findResult *cmsx.ItemDTO
	findErr    error

	upsertCalled struct {
		modelID, fieldKey, value string
		fields                   map[string]any
	}
	upsertResult *cmsx.ItemDTO
	upsertErr    error
}

func (s *stubItemClient) FindItemByFieldValue(_ context.Context, _, _, _ string) (*cmsx.ItemDTO, error) {
	return s.findResult, s.findErr
}

func (s *stubItemClient) UpsertItemByFieldValue(_ context.Context, modelID, fieldKey, value string, fields map[string]any) (*cmsx.ItemDTO, error) {
	s.upsertCalled.modelID = modelID
	s.upsertCalled.fieldKey = fieldKey
	s.upsertCalled.value = value
	s.upsertCalled.fields = fields
	return s.upsertResult, s.upsertErr
}

func sampleIncident() domain.SafetyIncident {
	now := time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC)
	return domain.Build(
		domain.MailItem{
			KeyCd: "k-1", Title: "title", InfoType: "spot",
			LeaveDate:   now,
			CountryCd:   "JP",
			CountryName: "日本",
		},
		domain.ExtractResult{Location: "東京", Confidence: 0.9},
		domain.GeocodeResult{
			Point:  domain.Point{Lat: 35.6, Lng: 139.7},
			Source: domain.GeocodeSourceMapbox,
		},
		now,
	)
}

func TestRepository_Exists_True(t *testing.T) {
	t.Parallel()
	stub := &stubItemClient{findResult: &cmsx.ItemDTO{ID: "i-1"}}
	r := cms.New(stub, "m-1", "key_cd")

	exists, err := r.Exists(context.Background(), "k-1")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !exists {
		t.Error("expected exists=true")
	}
}

func TestRepository_Exists_False(t *testing.T) {
	t.Parallel()
	stub := &stubItemClient{findResult: nil}
	r := cms.New(stub, "m-1", "key_cd")

	exists, err := r.Exists(context.Background(), "k-2")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if exists {
		t.Error("expected exists=false")
	}
}

func TestRepository_Exists_Error(t *testing.T) {
	t.Parallel()
	stub := &stubItemClient{findErr: errors.New("HTTP 500")}
	r := cms.New(stub, "m-1", "key_cd")

	_, err := r.Exists(context.Background(), "k-3")
	if err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestRepository_Exists_EmptyKey(t *testing.T) {
	t.Parallel()
	r := cms.New(&stubItemClient{}, "m-1", "key_cd")
	_, err := r.Exists(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty keyCd")
	}
	if !errs.IsKind(err, errs.KindInvalidInput) {
		t.Errorf("kind = %s, want KindInvalidInput", errs.KindOf(err))
	}
}

func TestRepository_Upsert_SendsAll19Fields(t *testing.T) {
	t.Parallel()
	stub := &stubItemClient{upsertResult: &cmsx.ItemDTO{ID: "i-new"}}
	r := cms.New(stub, "m-1", "key_cd")

	if err := r.Upsert(context.Background(), sampleIncident()); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	if stub.upsertCalled.modelID != "m-1" || stub.upsertCalled.fieldKey != "key_cd" || stub.upsertCalled.value != "k-1" {
		t.Errorf("call args = %+v", stub.upsertCalled)
	}
	want := []string{
		"key_cd", "info_type", "info_name", "leave_date", "title", "lead",
		"main_text", "info_url", "koukan_cd", "koukan_name", "area_cd",
		"area_name", "country_cd", "country_name", "extracted_location",
		"geometry", "geocode_source", "ingested_at", "updated_at",
	}
	if got := len(stub.upsertCalled.fields); got != len(want) {
		t.Errorf("fields count = %d, want %d", got, len(want))
	}
	for _, k := range want {
		if _, ok := stub.upsertCalled.fields[k]; !ok {
			t.Errorf("missing field %q", k)
		}
	}
	geom, ok := stub.upsertCalled.fields["geometry"].(map[string]any)
	if !ok || geom["type"] != "Point" {
		t.Errorf("geometry = %v, want GeoJSON Point", stub.upsertCalled.fields["geometry"])
	}
	coords, ok := geom["coordinates"].([]float64)
	if !ok || len(coords) != 2 || coords[0] != 139.7 || coords[1] != 35.6 {
		t.Errorf("coordinates = %v, want [139.7, 35.6]", geom["coordinates"])
	}
}

func TestRepository_Upsert_PropagatesError(t *testing.T) {
	t.Parallel()
	stub := &stubItemClient{upsertErr: errors.New("HTTP 503")}
	r := cms.New(stub, "m-1", "key_cd")
	if err := r.Upsert(context.Background(), sampleIncident()); err == nil {
		t.Fatal("expected error")
	}
}

func TestSizeForLog(t *testing.T) {
	t.Parallel()
	if got := cms.SizeForLog(sampleIncident()); got != "19" {
		t.Errorf("SizeForLog = %q, want 19", got)
	}
}
