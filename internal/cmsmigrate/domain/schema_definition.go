package domain

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

// aliasProjectRe and aliasFieldRe encode the naming rules enforced by
// reearth-cms. Project / Model aliases allow hyphens (kebab-case); Field
// aliases are snake_case to line up with the proto schema.
var (
	aliasProjectRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	aliasFieldRe   = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
)

// SchemaDefinition is the declarative description of a reearth-cms project
// plus its models and fields. It is the aggregate root for cmsmigrate: the
// use case walks it top-down and ensures the remote CMS matches.
type SchemaDefinition struct {
	Project ProjectDefinition
	Models  []ModelDefinition
}

// ProjectDefinition describes a top-level project container in reearth-cms.
type ProjectDefinition struct {
	Alias       string
	Name        string
	Description string
}

// ModelDefinition describes a data model that belongs to a project. The model
// owns its fields and declares which one serves as the upsert key.
type ModelDefinition struct {
	Alias         string
	Name          string
	Description   string
	KeyFieldAlias string
	Fields        []FieldDefinition
}

// FieldDefinition describes one field on a model. Unique = true means the CMS
// enforces a uniqueness constraint; cmsmigrate pairs it with Required = true
// on the model's key field so upserts by alias resolve deterministically.
type FieldDefinition struct {
	Alias       string
	Name        string
	Description string
	Type        FieldType
	Required    bool
	Unique      bool
	Multiple    bool
}

// Validate enforces invariants R1..R7 from U-CSS design §1.2.1. It returns a
// combined error (errors.Join) so callers can surface every violation on one
// run rather than a failure-at-a-time loop.
func (s SchemaDefinition) Validate() error {
	var violations []error

	// R1 — Project alias syntax
	if !aliasProjectRe.MatchString(s.Project.Alias) {
		violations = append(violations, fmt.Errorf(
			"R1: project alias %q does not match %s",
			s.Project.Alias, aliasProjectRe.String(),
		))
	}

	// R2 — at least one model
	if len(s.Models) == 0 {
		violations = append(violations, errors.New("R2: at least one model is required"))
	}

	// R3 — model aliases are unique within the schema
	modelAliases := make(map[string]struct{}, len(s.Models))
	for _, m := range s.Models {
		if _, dup := modelAliases[m.Alias]; dup {
			violations = append(violations, fmt.Errorf(
				"R3: model alias %q is duplicated", m.Alias,
			))
		}
		modelAliases[m.Alias] = struct{}{}
	}

	for _, m := range s.Models {
		violations = append(violations, validateModel(m)...)
	}

	if len(violations) == 0 {
		return nil
	}
	return errs.Wrap("cmsmigrate.domain.Validate", errs.KindInvalidInput, errors.Join(violations...))
}

func validateModel(m ModelDefinition) []error {
	var violations []error

	if !aliasProjectRe.MatchString(m.Alias) {
		violations = append(violations, fmt.Errorf(
			"R1: model alias %q does not match %s", m.Alias, aliasProjectRe.String(),
		))
	}

	// R4 — each model has at least one field
	if len(m.Fields) == 0 {
		violations = append(violations, fmt.Errorf("R4: model %q has no fields", m.Alias))
		return violations
	}

	// R5 — field aliases unique within a model + match snake_case regex
	// R6 — field type must be specified
	fieldAliases := make(map[string]FieldDefinition, len(m.Fields))
	for _, f := range m.Fields {
		if !aliasFieldRe.MatchString(f.Alias) {
			violations = append(violations, fmt.Errorf(
				"R5: field alias %q (model %q) does not match %s",
				f.Alias, m.Alias, aliasFieldRe.String(),
			))
		}
		if _, dup := fieldAliases[f.Alias]; dup {
			violations = append(violations, fmt.Errorf(
				"R5: field alias %q is duplicated in model %q", f.Alias, m.Alias,
			))
		}
		fieldAliases[f.Alias] = f

		if f.Type == FieldTypeUnspecified {
			violations = append(violations, fmt.Errorf(
				"R6: field %q (model %q) has no type", f.Alias, m.Alias,
			))
		}
	}

	// R7 — key field exists and is Required + Unique
	key, ok := fieldAliases[m.KeyFieldAlias]
	switch {
	case m.KeyFieldAlias == "":
		violations = append(violations, fmt.Errorf(
			"R7: model %q has no KeyFieldAlias", m.Alias,
		))
	case !ok:
		violations = append(violations, fmt.Errorf(
			"R7: model %q KeyFieldAlias %q is not in Fields", m.Alias, m.KeyFieldAlias,
		))
	case !key.Required || !key.Unique:
		violations = append(violations, fmt.Errorf(
			"R7: model %q key field %q must be Required and Unique (got required=%t unique=%t)",
			m.Alias, m.KeyFieldAlias, key.Required, key.Unique,
		))
	}

	return violations
}
