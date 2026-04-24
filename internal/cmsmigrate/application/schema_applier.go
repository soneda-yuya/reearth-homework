// Package application holds the cmsmigrate use case layer. It depends on
// [domain] and a [SchemaApplier] port whose implementations live in
// internal/cmsmigrate/infrastructure.
package application

import (
	"context"

	"github.com/soneda-yuya/overseas-safety-map/internal/cmsmigrate/domain"
)

// SchemaApplier is the outbound port the use case uses to reach the CMS.
// Implementations are responsible for returning (nil, nil) when a resource
// is not found (so "find" is a lookup, not an error signal).
//
// projectID / schemaID are threaded through because reearth-cms paths nest
// every model and field request under the owning project, and field creation
// specifically targets a schema (not the model directly). Carrying the IDs
// on the port keeps the HTTP layer stateless.
type SchemaApplier interface {
	FindProject(ctx context.Context, alias string) (*RemoteProject, error)
	CreateProject(ctx context.Context, def domain.ProjectDefinition) (*RemoteProject, error)

	FindModel(ctx context.Context, projectID, alias string) (*RemoteModel, error)
	CreateModel(ctx context.Context, projectID string, def domain.ModelDefinition) (*RemoteModel, error)

	FindField(ctx context.Context, projectID, modelID, alias string) (*RemoteField, error)
	CreateField(ctx context.Context, projectID, schemaID string, def domain.FieldDefinition) (*RemoteField, error)
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
// them inline); otherwise callers should rely on FindField. SchemaID is the
// CMS-side identifier of the model's schema; field create calls target it.
type RemoteModel struct {
	ID       string
	Alias    string
	Name     string
	SchemaID string
	Fields   []RemoteField
}

// RemoteField mirrors a CMS field. The boolean flags carry the CMS-side truth
// and are compared against [domain.FieldDefinition] to detect drift. Unique
// is kept as a domain concept even though the wire format does not transmit
// it: we know it from the declared definition and record it on the remote
// side only for drift-report parity (the wire response always reports false).
type RemoteField struct {
	ID       string
	Alias    string
	Type     domain.FieldType
	Required bool
	Unique   bool
	Multiple bool
}
