package cmsx

import (
	"fmt"

	"github.com/soneda-yuya/reearth-homework/internal/cmsmigrate/domain"
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

// ModelDTO mirrors one model under a project.
type ModelDTO struct {
	ID          string     `json:"id"`
	Alias       string     `json:"key"` // reearth-cms uses "key" as the model alias
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Fields      []FieldDTO `json:"schema,omitempty"`
}

// FieldDTO mirrors a single field in a model.
type FieldDTO struct {
	ID          string `json:"id"`
	Alias       string `json:"key"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Unique      bool   `json:"unique"`
	Multiple    bool   `json:"multiple"`
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

// createFieldBody is the POST payload for creating a field under a model.
type createFieldBody struct {
	Alias       string `json:"key"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Unique      bool   `json:"unique"`
	Multiple    bool   `json:"multiple"`
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
