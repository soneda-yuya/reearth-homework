package domain_test

import (
	"strings"
	"testing"

	"github.com/soneda-yuya/overseas-safety-map/internal/cmsmigrate/domain"
	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
	"pgregory.net/rapid"
)

// validSchema returns a schema that passes Validate. Individual tests mutate a
// copy to isolate one rule at a time.
func validSchema() domain.SchemaDefinition {
	return domain.SchemaDefinition{
		Project: domain.ProjectDefinition{Alias: "demo", Name: "Demo"},
		Models: []domain.ModelDefinition{
			{
				Alias:         "thing",
				Name:          "Thing",
				KeyFieldAlias: "id",
				Fields: []domain.FieldDefinition{
					{Alias: "id", Name: "ID", Type: domain.FieldTypeText, Required: true, Unique: true},
					{Alias: "label", Name: "Label", Type: domain.FieldTypeText},
				},
			},
		},
	}
}

func TestValidate_AcceptsValidSchema(t *testing.T) {
	t.Parallel()
	if err := validSchema().Validate(); err != nil {
		t.Fatalf("validSchema should pass Validate: %v", err)
	}
}

func TestValidate_RuleViolations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mutate   func(*domain.SchemaDefinition)
		wantText string // substring expected in error
	}{
		{
			name:     "R1 project alias uppercase",
			mutate:   func(s *domain.SchemaDefinition) { s.Project.Alias = "Demo" },
			wantText: "R1",
		},
		{
			name:     "R2 no models",
			mutate:   func(s *domain.SchemaDefinition) { s.Models = nil },
			wantText: "R2",
		},
		{
			name: "R3 duplicated model alias",
			mutate: func(s *domain.SchemaDefinition) {
				s.Models = append(s.Models, s.Models[0])
			},
			wantText: "R3",
		},
		{
			name: "R4 model without fields",
			mutate: func(s *domain.SchemaDefinition) {
				s.Models[0].Fields = nil
				s.Models[0].KeyFieldAlias = ""
			},
			wantText: "R4",
		},
		{
			name: "R5 field alias uppercase",
			mutate: func(s *domain.SchemaDefinition) {
				s.Models[0].Fields[0].Alias = "ID"
				s.Models[0].KeyFieldAlias = "ID"
			},
			wantText: "R5",
		},
		{
			name: "R5 duplicated field alias",
			mutate: func(s *domain.SchemaDefinition) {
				s.Models[0].Fields = append(s.Models[0].Fields, s.Models[0].Fields[0])
			},
			wantText: "R5",
		},
		{
			name: "R6 field type unspecified",
			mutate: func(s *domain.SchemaDefinition) {
				s.Models[0].Fields[1].Type = domain.FieldTypeUnspecified
			},
			wantText: "R6",
		},
		{
			name: "R6 field type out of range",
			mutate: func(s *domain.SchemaDefinition) {
				s.Models[0].Fields[1].Type = domain.FieldType(999)
			},
			wantText: "R6",
		},
		{
			name: "R7 missing key alias",
			mutate: func(s *domain.SchemaDefinition) {
				s.Models[0].KeyFieldAlias = "nope"
			},
			wantText: "R7",
		},
		{
			name: "R7 key not required",
			mutate: func(s *domain.SchemaDefinition) {
				s.Models[0].Fields[0].Required = false
			},
			wantText: "R7",
		},
		{
			name: "R7 key not unique",
			mutate: func(s *domain.SchemaDefinition) {
				s.Models[0].Fields[0].Unique = false
			},
			wantText: "R7",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := validSchema()
			tc.mutate(&s)
			err := s.Validate()
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantText)
			}
			if !errs.IsKind(err, errs.KindInvalidInput) {
				t.Fatalf("expected KindInvalidInput, got %s", errs.KindOf(err))
			}
			if !strings.Contains(err.Error(), tc.wantText) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantText)
			}
		})
	}
}

// TestValidate_Property uses PBT to exercise Validate with a broad random
// sample. The invariant we assert is one-directional: a schema that our
// generator declared valid MUST pass Validate. We do NOT claim the reverse
// (Validate-accepts → generator-accepts), because producing every edge case
// by random generation is wasteful.
func TestValidate_Property_GeneratedValidSchemaPasses(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		s := genValidSchema(t)
		if err := s.Validate(); err != nil {
			t.Fatalf("generated schema failed Validate: %v\nschema=%#v", err, s)
		}
	})
}

// TestValidate_Property_BrokenKeyAliasIsRejected picks a valid schema, then
// swaps its key alias for something not present in Fields. Validate must
// always reject this (R7). The mutation is cheap and reliable, so the test
// serves as a property-based anti-regression for R7.
func TestValidate_Property_BrokenKeyAliasIsRejected(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		s := genValidSchema(t)
		// Pick any model and break its key alias.
		idx := rapid.IntRange(0, len(s.Models)-1).Draw(t, "model_idx")
		s.Models[idx].KeyFieldAlias = "__not_a_real_field__"
		if err := s.Validate(); err == nil {
			t.Fatalf("expected R7 violation, got nil; schema=%#v", s)
		}
	})
}

// --- generators -----------------------------------------------------------

func genValidSchema(t *rapid.T) domain.SchemaDefinition {
	projectAlias := genProjectAlias(t, "p")
	modelCount := rapid.IntRange(1, 3).Draw(t, "model_count")

	models := make([]domain.ModelDefinition, 0, modelCount)
	seenModels := make(map[string]struct{}, modelCount)
	for i := 0; i < modelCount; i++ {
		var alias string
		for tries := 0; tries < 8; tries++ {
			alias = genProjectAlias(t, "m")
			if _, dup := seenModels[alias]; !dup {
				break
			}
		}
		if _, dup := seenModels[alias]; dup {
			// extremely unlikely but keep the generator total
			continue
		}
		seenModels[alias] = struct{}{}
		models = append(models, genValidModel(t, alias))
	}
	if len(models) == 0 {
		models = append(models, genValidModel(t, "fallback"))
	}

	return domain.SchemaDefinition{
		Project: domain.ProjectDefinition{Alias: projectAlias, Name: "P"},
		Models:  models,
	}
}

func genValidModel(t *rapid.T, alias string) domain.ModelDefinition {
	fieldCount := rapid.IntRange(1, 4).Draw(t, "field_count")
	fields := make([]domain.FieldDefinition, 0, fieldCount+1)
	seen := make(map[string]struct{}, fieldCount+1)

	// The key field alias is drawn from the *field* regex (snake_case) so the
	// generator is always valid even when the *model* alias contains a hyphen.
	keyAlias := "k_" + rapid.StringMatching(`[a-z0-9_]{0,8}`).Draw(t, "key_body")
	key := domain.FieldDefinition{
		Alias: keyAlias, Name: "K", Type: domain.FieldTypeText, Required: true, Unique: true,
	}
	fields = append(fields, key)
	seen[key.Alias] = struct{}{}

	for i := 0; i < fieldCount; i++ {
		var fa string
		for tries := 0; tries < 8; tries++ {
			fa = genFieldAlias(t)
			if _, dup := seen[fa]; !dup {
				break
			}
		}
		if _, dup := seen[fa]; dup {
			continue
		}
		seen[fa] = struct{}{}
		fields = append(fields, domain.FieldDefinition{
			Alias: fa,
			Name:  "F",
			Type:  pickFieldType(t),
		})
	}

	return domain.ModelDefinition{
		Alias:         alias,
		Name:          "M",
		KeyFieldAlias: key.Alias,
		Fields:        fields,
	}
}

func pickFieldType(t *rapid.T) domain.FieldType {
	// Exclude FieldTypeUnspecified; anything else is valid.
	choices := []domain.FieldType{
		domain.FieldTypeText,
		domain.FieldTypeTextArea,
		domain.FieldTypeURL,
		domain.FieldTypeDate,
		domain.FieldTypeSelect,
		domain.FieldTypeGeometryObject,
	}
	idx := rapid.IntRange(0, len(choices)-1).Draw(t, "field_type")
	return choices[idx]
}

// genProjectAlias emits `<prefix><kebab body>` — the prefix is a lowercase
// letter so the leading-char rule holds, and the body (possibly empty) is
// zero or more of `[a-z0-9-]`. The result always matches aliasProjectRe.
func genProjectAlias(t *rapid.T, prefix string) string {
	body := rapid.StringMatching(`[a-z0-9-]{0,12}`).Draw(t, prefix+"_body")
	return prefix + body
}

// genFieldAlias emits `f<lowercase alnum_...>` (that is, `f` followed by zero
// or more lowercase letters, digits, or underscores), which always matches
// the snake_case rule for fields.
func genFieldAlias(t *rapid.T) string {
	body := rapid.StringMatching(`[a-z0-9_]{0,12}`).Draw(t, "fa_body")
	return "f" + body
}
