package cmsx

import (
	"encoding/json"
	"fmt"

	"github.com/soneda-yuya/overseas-safety-map/internal/cmsmigrate/domain"
)

// ProjectDTO is the JSON shape cmsx returns for a project. Field names are
// kept in sync with the reearth-cms Integration REST API. Unknown fields are
// ignored so adding new attributes on the CMS side does not break decoding.
type ProjectDTO struct {
	ID          string `json:"id"`
	Alias       string `json:"alias"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// ModelDTO mirrors one model under a project. reearth-cms returns the schema
// as a nested object and exposes its ID as a top-level schemaId. We surface
// both plus the flattened Fields slice for callers that only care about
// attributes.
type ModelDTO struct {
	ID          string     `json:"id"`
	Alias       string     `json:"key"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	SchemaID    string     `json:"schemaId"`
	Fields      []FieldDTO `json:"-"`
}

// modelWire is the actual wire representation we decode into. Its only purpose
// is to absorb the nested schema.fields array before ModelDTO.UnmarshalJSON
// promotes it onto the flat ModelDTO.Fields.
type modelWire struct {
	ID          string `json:"id"`
	Alias       string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	SchemaID    string `json:"schemaId"`
	Schema      struct {
		Fields []FieldDTO `json:"fields"`
	} `json:"schema"`
}

// UnmarshalJSON flattens the nested schema.fields into ModelDTO.Fields so
// callers can treat fields as a direct property of the model without caring
// about the wire nesting. Unknown fields are tolerated by encoding/json.
func (m *ModelDTO) UnmarshalJSON(data []byte) error {
	var w modelWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	m.ID = w.ID
	m.Alias = w.Alias
	m.Name = w.Name
	m.Description = w.Description
	m.SchemaID = w.SchemaID
	m.Fields = w.Schema.Fields
	return nil
}

// FieldDTO mirrors a single field in a model's schema. reearth-cms exposes
// "key" as the alias and does not carry a description or a unique flag on the
// wire; those are omitted here so decoding stays faithful.
type FieldDTO struct {
	ID       string `json:"id"`
	Alias    string `json:"key"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
	Multiple bool   `json:"multiple"`
}

// createProjectBody is the POST payload for creating a project.
type createProjectBody struct {
	Alias       string `json:"alias"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// createModelBody is the POST payload for creating a model under a project.
type createModelBody struct {
	Alias       string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// createFieldBody is the POST payload for creating a field under a schema.
// reearth-cms only accepts type / key / required / multiple at create time;
// name and description are not part of the integration API contract, and
// unique isn't a wire concept (uniqueness is enforced via CMS model config,
// not per-field at the HTTP layer).
type createFieldBody struct {
	Type     string `json:"type"`
	Key      string `json:"key"`
	Required bool   `json:"required,omitempty"`
	Multiple bool   `json:"multiple,omitempty"`
}

// fieldTypeToAPI converts a domain.FieldType to the string used on the wire.
// The mapping is explicit rather than relying on FieldType.String so that a
// future edit to that method cannot silently change the API contract.
//
// Unsupported values panic on purpose: domain.SchemaDefinition.Validate is
// expected to reject them upstream (R6 + FieldType.IsValid), so reaching this
// branch indicates a programming error rather than user input.
func fieldTypeToAPI(t domain.FieldType) string {
	switch t {
	case domain.FieldTypeText:
		return "text"
	case domain.FieldTypeTextArea:
		return "textArea"
	case domain.FieldTypeURL:
		return "url"
	case domain.FieldTypeDate:
		return "date"
	case domain.FieldTypeSelect:
		return "select"
	case domain.FieldTypeGeometryObject:
		return "geometryObject"
	default:
		panic(fmt.Sprintf("cmsx: unsupported field type: %s", t))
	}
}

// fieldTypeFromAPI is the inverse; unknown strings return Unspecified so the
// use case treats them as drift rather than accepting them silently.
func fieldTypeFromAPI(s string) domain.FieldType {
	switch s {
	case "text":
		return domain.FieldTypeText
	case "textArea":
		return domain.FieldTypeTextArea
	case "url":
		return domain.FieldTypeURL
	case "date":
		return domain.FieldTypeDate
	case "select":
		return domain.FieldTypeSelect
	case "geometryObject":
		return domain.FieldTypeGeometryObject
	default:
		return domain.FieldTypeUnspecified
	}
}

// ToDomainType adapts a FieldDTO's wire type string into the domain's
// FieldType. Exported so the cmsclient adapter does not re-implement the
// translation table.
func (d FieldDTO) ToDomainType() domain.FieldType {
	return fieldTypeFromAPI(d.Type)
}

// apiError decorates the HTTP status for log / metric attribution.
type apiError struct {
	method string
	url    string
	status int
	body   string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("%s %s: HTTP %d: %s", e.method, e.url, e.status, truncate(e.body, 256))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}
