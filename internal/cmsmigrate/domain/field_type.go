// Package domain holds the schema declaration value objects used by cmsmigrate.
//
// The aggregate root is [SchemaDefinition]; [SafetyMapSchema] returns the
// canonical declaration used in production. All types here are pure data + a
// Validate method — no I/O, no goroutines.
package domain

import "fmt"

// FieldType enumerates the reearth-cms field types that cmsmigrate knows how
// to declare. Values are stable across serialisations (log / REST API) so new
// entries MUST be appended at the end.
type FieldType int

const (
	// FieldTypeUnspecified is the zero value and is always invalid. It exists
	// so that a missing Type in [FieldDefinition] fails Validate rather than
	// silently picking a default.
	FieldTypeUnspecified FieldType = iota
	// FieldTypeText is a short, single-line string.
	FieldTypeText
	// FieldTypeTextArea is a multi-line string.
	FieldTypeTextArea
	// FieldTypeURL is a URL-validated string.
	FieldTypeURL
	// FieldTypeDate maps to RFC3339 / google.protobuf.Timestamp on the proto side.
	FieldTypeDate
	// FieldTypeSelect is reserved for future enumerations. cmsmigrate does not
	// emit it today but the constant is declared so downstream code does not
	// have to recompute the enum order when it lands.
	FieldTypeSelect
	// FieldTypeGeometryObject carries GeoJSON-compatible geometry.
	FieldTypeGeometryObject
)

// String returns the canonical wire name. It is used both for logging and for
// translating to / from the reearth-cms REST API payloads, so the spelling
// matches the CMS documentation rather than Go idioms.
func (t FieldType) String() string {
	switch t {
	case FieldTypeText:
		return "text"
	case FieldTypeTextArea:
		return "textArea"
	case FieldTypeURL:
		return "url"
	case FieldTypeDate:
		return "date"
	case FieldTypeSelect:
		return "select"
	case FieldTypeGeometryObject:
		return "geometryObject"
	default:
		return fmt.Sprintf("unspecified(%d)", int(t))
	}
}
