package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/soneda-yuya/reearth-homework/internal/cmsmigrate/application"
	"github.com/soneda-yuya/reearth-homework/internal/cmsmigrate/domain"
	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

func TestExecute_FirstRun_CreatesEverything(t *testing.T) {
	t.Parallel()
	fake := newFakeApplier()
	usecase := application.NewEnsureSchemaUseCase(fake, nil, nil, nil)

	def := domain.SafetyMapSchema()
	res, err := usecase.Execute(context.Background(), application.EnsureSchemaInput{Definition: def})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res.ProjectCreated {
		t.Errorf("expected ProjectCreated=true")
	}
	if len(res.ModelsCreated) != 1 || res.ModelsCreated[0] != "safety-incident" {
		t.Errorf("ModelsCreated = %v, want [safety-incident]", res.ModelsCreated)
	}
	if len(res.FieldsCreated) != 19 {
		t.Errorf("FieldsCreated count = %d, want 19", len(res.FieldsCreated))
	}
	if len(res.DriftWarnings) != 0 {
		t.Errorf("DriftWarnings = %v, want empty", res.DriftWarnings)
	}
	if got := fake.countCalls("CreateField:"); got != 19 {
		t.Errorf("CreateField invocations = %d, want 19", got)
	}
}

func TestExecute_SecondRun_IsNoOp(t *testing.T) {
	t.Parallel()
	fake := newFakeApplier()
	usecase := application.NewEnsureSchemaUseCase(fake, nil, nil, nil)

	def := domain.SafetyMapSchema()
	if _, err := usecase.Execute(context.Background(), application.EnsureSchemaInput{Definition: def}); err != nil {
		t.Fatalf("first run: %v", err)
	}

	// Reset the breadcrumb so the second run's calls are easy to count.
	fake.calls = nil
	res, err := usecase.Execute(context.Background(), application.EnsureSchemaInput{Definition: def})
	if err != nil {
		t.Fatalf("second run: %v", err)
	}

	if res.ProjectCreated {
		t.Errorf("second run: expected ProjectCreated=false")
	}
	if len(res.ModelsCreated) != 0 || len(res.FieldsCreated) != 0 {
		t.Errorf("second run: expected no creations, got models=%v fields=%v",
			res.ModelsCreated, res.FieldsCreated)
	}
	if got := fake.countCalls("CreateField:"); got != 0 {
		t.Errorf("second run: CreateField invocations = %d, want 0", got)
	}
	if got := fake.countCalls("FindField:"); got != 19 {
		t.Errorf("second run: FindField invocations = %d, want 19", got)
	}
}

func TestExecute_ModelExists_CreatesFields(t *testing.T) {
	t.Parallel()
	fake := newFakeApplier()

	// Pre-populate Project + Model (but no fields) so the run has to fill in
	// the missing fields only.
	ctx := context.Background()
	def := domain.SafetyMapSchema()
	if _, err := fake.CreateProject(ctx, def.Project); err != nil {
		t.Fatalf("setup: %v", err)
	}
	projectID := fake.projects[def.Project.Alias].ID
	if _, err := fake.CreateModel(ctx, projectID, def.Models[0]); err != nil {
		t.Fatalf("setup: %v", err)
	}
	fake.calls = nil

	usecase := application.NewEnsureSchemaUseCase(fake, nil, nil, nil)
	res, err := usecase.Execute(ctx, application.EnsureSchemaInput{Definition: def})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	if res.ProjectCreated {
		t.Errorf("expected ProjectCreated=false")
	}
	if len(res.ModelsCreated) != 0 {
		t.Errorf("expected no new models, got %v", res.ModelsCreated)
	}
	if len(res.FieldsCreated) != 19 {
		t.Errorf("FieldsCreated count = %d, want 19", len(res.FieldsCreated))
	}
}

func TestExecute_FailFastOnCreateFieldError(t *testing.T) {
	t.Parallel()
	fake := newFakeApplier()
	// Fail on the 5th field ("title") to prove the loop stops there and later
	// fields are never attempted.
	fake.failCreateField = "title"
	fake.failErr = errs.Wrap("fake.CreateField", errs.KindExternal, errors.New("boom"))
	usecase := application.NewEnsureSchemaUseCase(fake, nil, nil, nil)

	def := domain.SafetyMapSchema()
	res, err := usecase.Execute(context.Background(), application.EnsureSchemaInput{Definition: def})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errs.IsKind(err, errs.KindExternal) {
		t.Fatalf("kind = %s, want %s", errs.KindOf(err), errs.KindExternal)
	}
	// 4 fields created before the failure.
	if got := len(res.FieldsCreated); got != 4 {
		t.Errorf("FieldsCreated = %d, want 4", got)
	}
	// CreateField was called for 5 fields (4 succeed, 1 fails), no more.
	if got := fake.countCalls("CreateField:"); got != 5 {
		t.Errorf("CreateField invocations = %d, want 5", got)
	}
}

func TestExecute_ProjectCreateConflict_RecoversByRefetch(t *testing.T) {
	t.Parallel()
	fake := newFakeApplier()
	def := domain.SafetyMapSchema()

	// Plant a racing project that "appeared" between FindProject and
	// CreateProject. The racing record carries a distinct ID we can assert
	// the use case picked up.
	fake.raceCreateProject = def.Project.Alias
	fake.raceCreateProjectSeed = &application.RemoteProject{
		ID: "p-raced", Alias: def.Project.Alias, Name: def.Project.Name,
	}

	usecase := application.NewEnsureSchemaUseCase(fake, nil, nil, nil)
	res, err := usecase.Execute(context.Background(), application.EnsureSchemaInput{Definition: def})
	if err != nil {
		t.Fatalf("expected recovery, got error: %v", err)
	}
	if res.ProjectCreated {
		t.Errorf("ProjectCreated must be false when create raced; got true")
	}
	// Models / fields should still land under the raced project ID so the
	// downstream FindModel call uses "p-raced/<alias>" as the lookup key.
	if _, ok := fake.models["p-raced/safety-incident"]; !ok {
		t.Errorf("model should have been created under the raced project ID")
	}
}

func TestExecute_ModelCreateConflict_RecoversByRefetch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	fake := newFakeApplier()
	def := domain.SafetyMapSchema()

	// Pre-create the project so the use case proceeds to model creation.
	if _, err := fake.CreateProject(ctx, def.Project); err != nil {
		t.Fatalf("setup project: %v", err)
	}
	projectID := fake.projects[def.Project.Alias].ID

	fake.raceCreateModel = def.Models[0].Alias
	fake.raceCreateModelSeed = &application.RemoteModel{
		ID: "m-raced", Alias: def.Models[0].Alias, Name: def.Models[0].Name,
	}
	fake.calls = nil

	usecase := application.NewEnsureSchemaUseCase(fake, nil, nil, nil)
	res, err := usecase.Execute(ctx, application.EnsureSchemaInput{Definition: def})
	if err != nil {
		t.Fatalf("expected recovery, got error: %v", err)
	}
	if len(res.ModelsCreated) != 0 {
		t.Errorf("ModelsCreated must be empty on race; got %v", res.ModelsCreated)
	}
	// 19 fields created under the raced model ID.
	if got := fake.countCalls("CreateField:m-raced/"); got != 19 {
		t.Errorf("expected 19 CreateField calls under raced model, got %d", got)
	}
	if _ = projectID; len(res.FieldsCreated) != 19 {
		t.Errorf("FieldsCreated count = %d, want 19", len(res.FieldsCreated))
	}
}

func TestExecute_FieldCreateConflict_RecordsDriftIfShapeDiffers(t *testing.T) {
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

	// Pre-create the key field so we can target the race on "body".
	fake.fields[modelID+"/id"] = &application.RemoteField{
		ID: "f-id", Alias: "id", Type: domain.FieldTypeText, Required: true, Unique: true,
	}
	// Plant a raced "body" field with a *drifting* type (Text instead of TextArea).
	fake.raceCreateField = "body"
	fake.raceCreateFieldSeed = &application.RemoteField{
		ID: "f-body-raced", Alias: "body", Type: domain.FieldTypeText,
	}
	fake.calls = nil

	usecase := application.NewEnsureSchemaUseCase(fake, nil, nil, nil)
	res, err := usecase.Execute(ctx, application.EnsureSchemaInput{Definition: def})
	if err != nil {
		t.Fatalf("expected recovery, got error: %v", err)
	}
	if len(res.FieldsCreated) != 0 {
		t.Errorf("FieldsCreated must be empty on race; got %v", res.FieldsCreated)
	}
	if len(res.DriftWarnings) != 1 {
		t.Fatalf("expected 1 drift warning (raced field shape mismatch), got %d", len(res.DriftWarnings))
	}
	if res.DriftWarnings[0].Resource != "Field:m.body" {
		t.Errorf("drift Resource = %q", res.DriftWarnings[0].Resource)
	}
}

func TestExecute_ValidateFailsWithInvalidDefinition(t *testing.T) {
	t.Parallel()
	fake := newFakeApplier()
	usecase := application.NewEnsureSchemaUseCase(fake, nil, nil, nil)

	bad := domain.SchemaDefinition{Project: domain.ProjectDefinition{Alias: "Bad"}}
	_, err := usecase.Execute(context.Background(), application.EnsureSchemaInput{Definition: bad})
	if err == nil {
		t.Fatalf("expected validation error, got nil")
	}
	if !errs.IsKind(err, errs.KindInvalidInput) {
		t.Fatalf("kind = %s, want %s", errs.KindOf(err), errs.KindInvalidInput)
	}
	if got := fake.countCalls("FindProject:"); got != 0 {
		t.Errorf("FindProject should not be called when validation fails, got %d calls", got)
	}
}
