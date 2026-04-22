package application_test

import (
	"context"
	"strings"
	"testing"

	"github.com/soneda-yuya/reearth-homework/internal/cmsmigrate/application"
	"github.com/soneda-yuya/reearth-homework/internal/cmsmigrate/domain"
)

// TestExecute_DriftReportedForTypeMismatch seeds the fake with a field whose
// type differs from the declaration, then checks that the use case reports
// drift (one warning) without attempting to overwrite the CMS value.
func TestExecute_DriftReportedForTypeMismatch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fake := newFakeApplier()

	def := domain.SchemaDefinition{
		Project: domain.ProjectDefinition{Alias: "demo", Name: "D"},
		Models: []domain.ModelDefinition{{
			Alias:         "m",
			Name:          "M",
			KeyFieldAlias: "id",
			Fields: []domain.FieldDefinition{
				{Alias: "id", Name: "ID", Type: domain.FieldTypeText, Required: true, Unique: true},
				{Alias: "body", Name: "Body", Type: domain.FieldTypeTextArea},
			},
		}},
	}

	if _, err := fake.CreateProject(ctx, def.Project); err != nil {
		t.Fatalf("setup: %v", err)
	}
	projectID := fake.projects[def.Project.Alias].ID
	if _, err := fake.CreateModel(ctx, projectID, def.Models[0]); err != nil {
		t.Fatalf("setup: %v", err)
	}
	modelID := fake.models[projectID+"/m"].ID

	// Seed an "id" field with the correct shape so R7 passes, and a "body"
	// field whose type drifts (Text instead of TextArea).
	fake.fields[modelID+"/id"] = &application.RemoteField{
		ID: "f-x", Alias: "id", Type: domain.FieldTypeText, Required: true, Unique: true,
	}
	fake.fields[modelID+"/body"] = &application.RemoteField{
		ID: "f-y", Alias: "body", Type: domain.FieldTypeText, // drift
	}

	usecase := application.NewEnsureSchemaUseCase(fake, nil, nil, nil)
	res, err := usecase.Execute(ctx, application.EnsureSchemaInput{Definition: def})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.FieldsCreated) != 0 {
		t.Errorf("expected no creations, got %v", res.FieldsCreated)
	}
	if len(res.DriftWarnings) != 1 {
		t.Fatalf("DriftWarnings = %v, want 1", res.DriftWarnings)
	}
	w := res.DriftWarnings[0]
	if w.Resource != "Field:m.body" {
		t.Errorf("drift.Resource = %q, want Field:m.body", w.Resource)
	}
	if !strings.Contains(w.Reason, "type=text") {
		t.Errorf("drift.Reason = %q, expected to mention type mismatch", w.Reason)
	}
}

// TestExecute_DriftReportedForFlagMismatch checks that required / unique /
// multiple mismatches each surface in the reason string.
func TestExecute_DriftReportedForFlagMismatch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fake := newFakeApplier()

	def := domain.SchemaDefinition{
		Project: domain.ProjectDefinition{Alias: "demo", Name: "D"},
		Models: []domain.ModelDefinition{{
			Alias:         "m",
			Name:          "M",
			KeyFieldAlias: "id",
			Fields: []domain.FieldDefinition{
				{Alias: "id", Name: "ID", Type: domain.FieldTypeText, Required: true, Unique: true},
				{Alias: "name", Name: "N", Type: domain.FieldTypeText, Required: true, Multiple: true},
			},
		}},
	}
	if _, err := fake.CreateProject(ctx, def.Project); err != nil {
		t.Fatalf("setup: %v", err)
	}
	projectID := fake.projects[def.Project.Alias].ID
	if _, err := fake.CreateModel(ctx, projectID, def.Models[0]); err != nil {
		t.Fatalf("setup: %v", err)
	}
	modelID := fake.models[projectID+"/m"].ID
	fake.fields[modelID+"/id"] = &application.RemoteField{
		ID: "f-x", Alias: "id", Type: domain.FieldTypeText, Required: true, Unique: true,
	}
	fake.fields[modelID+"/name"] = &application.RemoteField{
		ID: "f-y", Alias: "name", Type: domain.FieldTypeText,
		Required: false, Multiple: false, // drift: required + multiple
	}

	usecase := application.NewEnsureSchemaUseCase(fake, nil, nil, nil)
	res, err := usecase.Execute(ctx, application.EnsureSchemaInput{Definition: def})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.DriftWarnings) != 1 {
		t.Fatalf("DriftWarnings count = %d, want 1", len(res.DriftWarnings))
	}
	reason := res.DriftWarnings[0].Reason
	if !strings.Contains(reason, "required=false") || !strings.Contains(reason, "multiple=false") {
		t.Errorf("drift reason = %q, expected both required and multiple mentions", reason)
	}
}
