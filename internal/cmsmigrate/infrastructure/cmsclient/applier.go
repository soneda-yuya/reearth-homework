// Package cmsclient is the infrastructure adapter that fulfils
// application.SchemaApplier by delegating to cmsx.Client. The translation is
// mostly mechanical; it lives here instead of in the use case so domain code
// stays free of HTTP-shaped types.
package cmsclient

import (
	"context"

	"github.com/soneda-yuya/overseas-safety-map/internal/cmsmigrate/application"
	"github.com/soneda-yuya/overseas-safety-map/internal/cmsmigrate/domain"
	"github.com/soneda-yuya/overseas-safety-map/internal/platform/cmsx"
)

// SchemaClient is the subset of cmsx.Client the adapter needs. Keeping it as
// an interface lets tests substitute an in-memory stub without spinning up a
// real HTTP server.
type SchemaClient interface {
	FindProjectByAlias(ctx context.Context, alias string) (*cmsx.ProjectDTO, error)
	CreateProject(ctx context.Context, def domain.ProjectDefinition) (*cmsx.ProjectDTO, error)

	FindModelByAlias(ctx context.Context, projectID, alias string) (*cmsx.ModelDTO, error)
	CreateModel(ctx context.Context, projectID string, def domain.ModelDefinition) (*cmsx.ModelDTO, error)

	FindFieldByAlias(ctx context.Context, projectID, modelID, alias string) (*cmsx.FieldDTO, error)
	CreateField(ctx context.Context, projectID, schemaID string, def domain.FieldDefinition) (*cmsx.FieldDTO, error)
}

// CMSSchemaApplier implements application.SchemaApplier against cmsx.Client.
type CMSSchemaApplier struct {
	client SchemaClient
}

// New constructs a CMSSchemaApplier backed by a real cmsx.Client; tests
// typically use NewWithClient to inject a stub.
func New(c *cmsx.Client) *CMSSchemaApplier {
	return &CMSSchemaApplier{client: c}
}

// NewWithClient accepts any SchemaClient, mainly for testing.
func NewWithClient(c SchemaClient) *CMSSchemaApplier {
	return &CMSSchemaApplier{client: c}
}

// FindProject forwards to cmsx and translates the DTO into the
// application-layer shape.
func (a *CMSSchemaApplier) FindProject(ctx context.Context, alias string) (*application.RemoteProject, error) {
	dto, err := a.client.FindProjectByAlias(ctx, alias)
	if err != nil || dto == nil {
		return nil, err
	}
	return projectDTOTo(dto), nil
}

func (a *CMSSchemaApplier) CreateProject(ctx context.Context, def domain.ProjectDefinition) (*application.RemoteProject, error) {
	dto, err := a.client.CreateProject(ctx, def)
	if err != nil {
		return nil, err
	}
	return projectDTOTo(dto), nil
}

func (a *CMSSchemaApplier) FindModel(ctx context.Context, projectID, alias string) (*application.RemoteModel, error) {
	dto, err := a.client.FindModelByAlias(ctx, projectID, alias)
	if err != nil || dto == nil {
		return nil, err
	}
	return modelDTOTo(dto), nil
}

func (a *CMSSchemaApplier) CreateModel(ctx context.Context, projectID string, def domain.ModelDefinition) (*application.RemoteModel, error) {
	dto, err := a.client.CreateModel(ctx, projectID, def)
	if err != nil {
		return nil, err
	}
	return modelDTOTo(dto), nil
}

func (a *CMSSchemaApplier) FindField(ctx context.Context, projectID, modelID, alias string) (*application.RemoteField, error) {
	dto, err := a.client.FindFieldByAlias(ctx, projectID, modelID, alias)
	if err != nil || dto == nil {
		return nil, err
	}
	return fieldDTOTo(dto), nil
}

func (a *CMSSchemaApplier) CreateField(ctx context.Context, projectID, schemaID string, def domain.FieldDefinition) (*application.RemoteField, error) {
	dto, err := a.client.CreateField(ctx, projectID, schemaID, def)
	if err != nil {
		return nil, err
	}
	return fieldDTOTo(dto), nil
}

func projectDTOTo(dto *cmsx.ProjectDTO) *application.RemoteProject {
	return &application.RemoteProject{ID: dto.ID, Alias: dto.Alias, Name: dto.Name}
}

func modelDTOTo(dto *cmsx.ModelDTO) *application.RemoteModel {
	out := &application.RemoteModel{
		ID:       dto.ID,
		Alias:    dto.Alias,
		Name:     dto.Name,
		SchemaID: dto.SchemaID,
	}
	// Iterate by index and take the slice element address explicitly. Even
	// with Go 1.22's per-iteration loop variable, &elem in a `for _, elem`
	// is a recurring source of bugs in code review, so we avoid the shape.
	for i := range dto.Fields {
		out.Fields = append(out.Fields, *fieldDTOTo(&dto.Fields[i]))
	}
	return out
}

func fieldDTOTo(dto *cmsx.FieldDTO) *application.RemoteField {
	return &application.RemoteField{
		ID:       dto.ID,
		Alias:    dto.Alias,
		Type:     dto.ToDomainType(),
		Required: dto.Required,
		// Unique is intentionally left as the zero value: reearth-cms's
		// Integration API does not surface the unique flag on field GET,
		// so we cannot observe the server-side truth. detectFieldDrift
		// compensates by skipping the unique comparison, preventing a
		// false-positive drift warning for every Unique-declared field
		// (notably key_cd).
		Unique:   false,
		Multiple: dto.Multiple,
	}
}
