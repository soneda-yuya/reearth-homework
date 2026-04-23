package domain_test

import (
	"testing"

	"github.com/soneda-yuya/reearth-homework/internal/cmsmigrate/domain"
)

func TestSafetyMapSchema_PassesValidate(t *testing.T) {
	t.Parallel()
	if err := domain.SafetyMapSchema().Validate(); err != nil {
		t.Fatalf("SafetyMapSchema() should pass Validate: %v", err)
	}
}

// TestSafetyMapSchema_MatchesProto is an anti-regression: the 19 field names
// and types are copied from proto/v1/safetymap.proto. If someone edits one
// without the other this test points at the diff.
func TestSafetyMapSchema_MatchesProto(t *testing.T) {
	t.Parallel()
	s := domain.SafetyMapSchema()
	if got, want := s.Project.Alias, "overseas-safety-map"; got != want {
		t.Fatalf("project alias: got %q want %q", got, want)
	}
	if got, want := len(s.Models), 1; got != want {
		t.Fatalf("model count: got %d want %d", got, want)
	}
	m := s.Models[0]
	if got, want := m.Alias, "safety-incident"; got != want {
		t.Fatalf("model alias: got %q want %q", got, want)
	}
	if got, want := m.KeyFieldAlias, "key_cd"; got != want {
		t.Fatalf("key alias: got %q want %q", got, want)
	}

	want := []struct {
		alias    string
		ftype    domain.FieldType
		required bool
		unique   bool
	}{
		{"key_cd", domain.FieldTypeText, true, true},
		{"info_type", domain.FieldTypeText, true, false},
		{"info_name", domain.FieldTypeText, false, false},
		{"leave_date", domain.FieldTypeDate, true, false},
		{"title", domain.FieldTypeText, true, false},
		{"lead", domain.FieldTypeTextArea, false, false},
		{"main_text", domain.FieldTypeTextArea, false, false},
		{"info_url", domain.FieldTypeURL, false, false},
		{"koukan_cd", domain.FieldTypeText, false, false},
		{"koukan_name", domain.FieldTypeText, false, false},
		{"area_cd", domain.FieldTypeText, false, false},
		{"area_name", domain.FieldTypeText, false, false},
		{"country_cd", domain.FieldTypeText, true, false},
		{"country_name", domain.FieldTypeText, false, false},
		{"extracted_location", domain.FieldTypeText, false, false},
		{"geometry", domain.FieldTypeGeometryObject, false, false},
		{"geocode_source", domain.FieldTypeText, false, false},
		{"ingested_at", domain.FieldTypeDate, true, false},
		{"updated_at", domain.FieldTypeDate, true, false},
	}

	if got := len(m.Fields); got != len(want) {
		t.Fatalf("field count: got %d want %d", got, len(want))
	}
	for i, w := range want {
		f := m.Fields[i]
		if f.Alias != w.alias {
			t.Errorf("field[%d] alias: got %q want %q", i, f.Alias, w.alias)
		}
		if f.Type != w.ftype {
			t.Errorf("field[%d] %q type: got %s want %s", i, f.Alias, f.Type, w.ftype)
		}
		if f.Required != w.required {
			t.Errorf("field[%d] %q required: got %t want %t", i, f.Alias, f.Required, w.required)
		}
		if f.Unique != w.unique {
			t.Errorf("field[%d] %q unique: got %t want %t", i, f.Alias, f.Unique, w.unique)
		}
	}
}

func TestFieldType_IsValid(t *testing.T) {
	t.Parallel()
	valid := []domain.FieldType{
		domain.FieldTypeText,
		domain.FieldTypeTextArea,
		domain.FieldTypeURL,
		domain.FieldTypeDate,
		domain.FieldTypeSelect,
		domain.FieldTypeGeometryObject,
	}
	for _, v := range valid {
		if !v.IsValid() {
			t.Errorf("FieldType(%d) should be valid", int(v))
		}
	}
	invalid := []domain.FieldType{
		domain.FieldTypeUnspecified,
		domain.FieldType(999),
		domain.FieldType(-1),
	}
	for _, v := range invalid {
		if v.IsValid() {
			t.Errorf("FieldType(%d) should be invalid", int(v))
		}
	}
}

func TestFieldType_String(t *testing.T) {
	t.Parallel()
	tests := []struct {
		t    domain.FieldType
		want string
	}{
		{domain.FieldTypeText, "text"},
		{domain.FieldTypeTextArea, "textArea"},
		{domain.FieldTypeURL, "url"},
		{domain.FieldTypeDate, "date"},
		{domain.FieldTypeSelect, "select"},
		{domain.FieldTypeGeometryObject, "geometryObject"},
	}
	for _, tc := range tests {
		if got := tc.t.String(); got != tc.want {
			t.Errorf("FieldType(%d).String() = %q, want %q", int(tc.t), got, tc.want)
		}
	}
	if got := domain.FieldTypeUnspecified.String(); got == "" {
		t.Errorf("FieldTypeUnspecified.String() should not be empty")
	}
}
