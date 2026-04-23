package cmsclient_test

import (
	"context"
	"testing"

	"github.com/soneda-yuya/overseas-safety-map/internal/cmsmigrate/domain"
	"github.com/soneda-yuya/overseas-safety-map/internal/cmsmigrate/infrastructure/cmsclient"
	"github.com/soneda-yuya/overseas-safety-map/internal/platform/cmsx"
)

// stubClient is the minimum cmsx-shaped surface the adapter uses. Returning
// canned DTOs lets us exercise the translation layer without http.
type stubClient struct {
	projects map[string]*cmsx.ProjectDTO
	models   map[string]*cmsx.ModelDTO
	fields   map[string]*cmsx.FieldDTO
}

func newStub() *stubClient {
	return &stubClient{
		projects: map[string]*cmsx.ProjectDTO{},
		models:   map[string]*cmsx.ModelDTO{},
		fields:   map[string]*cmsx.FieldDTO{},
	}
}

func (s *stubClient) FindProjectByAlias(_ context.Context, alias string) (*cmsx.ProjectDTO, error) {
	return s.projects[alias], nil
}

func (s *stubClient) CreateProject(_ context.Context, def domain.ProjectDefinition) (*cmsx.ProjectDTO, error) {
	p := &cmsx.ProjectDTO{ID: "p-" + def.Alias, Alias: def.Alias, Name: def.Name}
	s.projects[def.Alias] = p
	return p, nil
}

func (s *stubClient) FindModelByAlias(_ context.Context, projectID, alias string) (*cmsx.ModelDTO, error) {
	return s.models[projectID+"/"+alias], nil
}

func (s *stubClient) CreateModel(_ context.Context, projectID string, def domain.ModelDefinition) (*cmsx.ModelDTO, error) {
	m := &cmsx.ModelDTO{ID: "m-" + def.Alias, Alias: def.Alias, Name: def.Name}
	s.models[projectID+"/"+def.Alias] = m
	return m, nil
}

func (s *stubClient) FindFieldByAlias(_ context.Context, modelID, alias string) (*cmsx.FieldDTO, error) {
	return s.fields[modelID+"/"+alias], nil
}

func (s *stubClient) CreateField(_ context.Context, modelID string, def domain.FieldDefinition) (*cmsx.FieldDTO, error) {
	// FieldType.String() is the canonical wire name (same one cmsx serialises
	// over HTTP), so the stub piggybacks on it instead of duplicating the
	// mapping table.
	dto := &cmsx.FieldDTO{
		ID: "f-" + def.Alias, Alias: def.Alias, Type: def.Type.String(),
		Required: def.Required, Unique: def.Unique, Multiple: def.Multiple,
	}
	s.fields[modelID+"/"+def.Alias] = dto
	return dto, nil
}

func TestAdapter_FindProject_MissReturnsNilNil(t *testing.T) {
	t.Parallel()
	a := cmsclient.NewWithClient(newStub())
	got, err := a.FindProject(context.Background(), "missing")
	if err != nil || got != nil {
		t.Fatalf("got (%+v, %v), want (nil, nil)", got, err)
	}
}

func TestAdapter_CreateProjectAndReadBack(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := newStub()
	a := cmsclient.NewWithClient(s)

	created, err := a.CreateProject(ctx, domain.ProjectDefinition{Alias: "demo", Name: "Demo"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if created.Alias != "demo" || created.ID == "" {
		t.Errorf("created = %+v", created)
	}
	got, err := a.FindProject(ctx, "demo")
	if err != nil {
		t.Fatalf("FindProject: %v", err)
	}
	if got == nil || got.ID != created.ID {
		t.Errorf("round-trip mismatch: %+v vs %+v", got, created)
	}
}

func TestAdapter_CreateFieldTranslatesTypes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := newStub()
	a := cmsclient.NewWithClient(s)

	got, err := a.CreateField(ctx, "m-1", domain.FieldDefinition{
		Alias: "geom", Name: "Geometry", Type: domain.FieldTypeGeometryObject, Required: false,
	})
	if err != nil {
		t.Fatalf("CreateField: %v", err)
	}
	if got.Type != domain.FieldTypeGeometryObject {
		t.Errorf("got.Type = %s", got.Type)
	}
	// And the stub kept the API-serialised form.
	if s.fields["m-1/geom"].Type != "geometryObject" {
		t.Errorf("stub stored wire type %q", s.fields["m-1/geom"].Type)
	}
}

func TestAdapter_CreateModelAndFindField(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := newStub()
	a := cmsclient.NewWithClient(s)

	m, err := a.CreateModel(ctx, "p-1", domain.ModelDefinition{
		Alias: "thing", Name: "Thing",
	})
	if err != nil || m == nil || m.Alias != "thing" {
		t.Fatalf("CreateModel: got %+v err %v", m, err)
	}

	// Seed a field on the stub and read it back via FindField.
	s.fields["m-1/id"] = &cmsx.FieldDTO{
		ID: "f-1", Alias: "id", Type: "text", Required: true, Unique: true,
	}
	got, err := a.FindField(ctx, "m-1", "id")
	if err != nil || got == nil {
		t.Fatalf("FindField: got %+v err %v", got, err)
	}
	if got.Type != domain.FieldTypeText || !got.Required || !got.Unique {
		t.Errorf("FindField unexpected shape: %+v", got)
	}

	miss, err := a.FindField(ctx, "m-1", "nope")
	if err != nil || miss != nil {
		t.Errorf("expected (nil,nil), got (%+v, %v)", miss, err)
	}
}

// TestNew_AcceptsRealCmsxClient is a smoke test that New returns a usable
// adapter when wired with the concrete cmsx.Client. We do not exercise a
// real HTTP round-trip here (see schema_test.go in the cmsx package).
func TestNew_AcceptsRealCmsxClient(t *testing.T) {
	t.Parallel()
	c := cmsx.NewClient(cmsx.Config{BaseURL: "http://localhost", WorkspaceID: "ws", Token: "t"})
	a := cmsclient.New(c)
	if a == nil {
		t.Fatal("New returned nil")
	}
}

func TestAdapter_FindModelIncludesFields(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := newStub()
	s.models["p-1/thing"] = &cmsx.ModelDTO{
		ID: "m-1", Alias: "thing", Name: "Thing",
		Fields: []cmsx.FieldDTO{
			{ID: "f-1", Alias: "id", Type: "text", Required: true, Unique: true},
			{ID: "f-2", Alias: "body", Type: "textArea"},
		},
	}
	a := cmsclient.NewWithClient(s)

	m, err := a.FindModel(ctx, "p-1", "thing")
	if err != nil {
		t.Fatalf("FindModel: %v", err)
	}
	if m == nil || len(m.Fields) != 2 {
		t.Fatalf("got %+v", m)
	}
	if m.Fields[1].Type != domain.FieldTypeTextArea {
		t.Errorf("field[1].Type = %s", m.Fields[1].Type)
	}
}
