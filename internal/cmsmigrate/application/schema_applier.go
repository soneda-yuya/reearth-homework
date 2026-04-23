// Package application holds the cmsmigrate use case layer. It depends on
// [domain] and a [SchemaApplier] port whose implementations live in
// internal/cmsmigrate/infrastructure.
package application

import (
	"context"

	"github.com/soneda-yuya/reearth-homework/internal/cmsmigrate/domain"
)

// SchemaApplier is the outbound port the use case uses to reach the CMS.
// Implementations are responsible for returning (nil, nil) when a resource
// is not found (so "find" is a lookup, not an error signal).
type SchemaApplier interface {
	FindProject(ctx context.Context, alias string) (*RemoteProject, error)
	CreateProject(ctx context.Context, def domain.ProjectDefinition) (*RemoteProject, error)

	FindModel(ctx context.Context, projectID, alias string) (*RemoteModel, error)
	CreateModel(ctx context.Context, projectID string, def domain.ModelDefinition) (*RemoteModel, error)

	FindField(ctx context.Context, modelID, alias string) (*RemoteField, error)
	CreateField(ctx context.Context, modelID string, def domain.FieldDefinition) (*RemoteField, error)
}

// RemoteProject is the adapter-independent view of a reearth-cms project.
// IDs are opaque strings (UUIDs on the API side) used only to chain the next
// FindModel / CreateModel call.
type RemoteProject struct {
	ID    string
	Alias string
	Name  string
}

// RemoteModel mirrors a CMS model. Fields are only populated when the adapter
// cheaply fetched them alongside the model (e.g. when the REST API returns
// them inline); otherwise callers should rely on FindField.
type RemoteModel struct {
	ID     string
	Alias  string
	Name   string
	Fields []RemoteField
}

// RemoteField mirrors a CMS field. The boolean flags carry the CMS-side truth
// and are compared against [domain.FieldDefinition] to detect drift.
type RemoteField struct {
	ID       string
	Alias    string
	Type     domain.FieldType
	Required bool
	Unique   bool
	Multiple bool
}
