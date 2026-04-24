package domain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/domain"
	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
)

func validMailItem() domain.MailItem {
	return domain.MailItem{
		KeyCd:     "MOFA-2026-0001",
		Title:     "渡航中止勧告",
		LeaveDate: time.Date(2026, 4, 23, 12, 0, 0, 0, time.UTC),
		CountryCd: "JP",
	}
}

func TestMailItem_Validate_Pass(t *testing.T) {
	t.Parallel()
	if err := validMailItem().Validate(); err != nil {
		t.Fatalf("validMailItem should pass: %v", err)
	}
}

func TestMailItem_Validate_Failures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		mutate   func(*domain.MailItem)
		wantText string
	}{
		{"empty key_cd", func(m *domain.MailItem) { m.KeyCd = "" }, "key_cd"},
		{"zero leave_date", func(m *domain.MailItem) { m.LeaveDate = time.Time{} }, "leave_date"},
		{"empty title", func(m *domain.MailItem) { m.Title = "" }, "title"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := validMailItem()
			tc.mutate(&m)
			err := m.Validate()
			if err == nil {
				t.Fatalf("expected error mentioning %q", tc.wantText)
			}
			if !errs.IsKind(err, errs.KindInvalidInput) {
				t.Errorf("kind = %s, want %s", errs.KindOf(err), errs.KindInvalidInput)
			}
			if !strings.Contains(err.Error(), tc.wantText) {
				t.Errorf("error %q does not contain %q", err, tc.wantText)
			}
		})
	}
}

// TestMailItem_Validate_EmptyCountryCdIsAllowed documents the design: MOFA
// occasionally ships items without a nested <country> element (global
// advisories, sample entries). Validation lets them through and leaves the
// geocoder chain to backfill country_cd from Mapbox — items that can't be
// placed at all are dropped later by the use case, not here.
func TestMailItem_Validate_EmptyCountryCdIsAllowed(t *testing.T) {
	t.Parallel()
	m := validMailItem()
	m.CountryCd = ""
	if err := m.Validate(); err != nil {
		t.Fatalf("empty country_cd must pass validation (geocoder backfills): %v", err)
	}
}

func TestMailItem_Validate_AggregatesViolations(t *testing.T) {
	t.Parallel()
	m := domain.MailItem{} // every required field empty
	err := m.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"key_cd", "leave_date", "title"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("expected aggregated error to contain %q, got %q", want, err)
		}
	}
}
