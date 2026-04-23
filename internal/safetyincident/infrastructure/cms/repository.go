// Package cms is the safetyincident persistence adapter. It bridges
// domain.Repository onto a CMS Item via cmsx.Client; the model id is
// resolved at startup so the hot path doesn't need to discover it on every
// upsert.
package cms

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/soneda-yuya/overseas-safety-map/internal/platform/cmsx"
	"github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/domain"
	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
)

// ItemClient is the subset of cmsx.Client this adapter calls. Declared as
// an interface so the test can swap a stub without setting up an HTTP
// server.
type ItemClient interface {
	FindItemByFieldValue(ctx context.Context, modelID, fieldKey, value string) (*cmsx.ItemDTO, error)
	UpsertItemByFieldValue(ctx context.Context, modelID, fieldKey, value string, fields map[string]any) (*cmsx.ItemDTO, error)
}

// Repository fulfils domain.Repository against reearth-cms. modelID is the
// numeric/UUID resolved by U-CSS during schema apply; fieldKey is the
// alias of the unique key field (typically "key_cd").
type Repository struct {
	client   ItemClient
	modelID  string
	keyField string
}

// New returns a Repository wired to the given Model.
func New(client ItemClient, modelID, keyField string) *Repository {
	return &Repository{client: client, modelID: modelID, keyField: keyField}
}

// Exists reports whether the CMS already has an item with the given key.
// Used by the use case to short-circuit LLM/Mapbox calls (Q3 [A]).
func (r *Repository) Exists(ctx context.Context, keyCd string) (bool, error) {
	if keyCd == "" {
		return false, errs.Wrap("cms.repository.exists",
			errs.KindInvalidInput,
			errors.New("key_cd is required"))
	}
	dto, err := r.client.FindItemByFieldValue(ctx, r.modelID, r.keyField, keyCd)
	if err != nil {
		return false, errs.Wrap("cms.repository.exists", errs.KindOf(err), err)
	}
	return dto != nil, nil
}

// Upsert writes the SafetyIncident into CMS, creating or updating by KeyCd.
// All 19 fields are sent on every upsert — partial updates are not worth
// the diff complexity at this scale.
func (r *Repository) Upsert(ctx context.Context, incident domain.SafetyIncident) error {
	fields := toFields(incident)
	if _, err := r.client.UpsertItemByFieldValue(ctx, r.modelID, r.keyField, incident.KeyCd, fields); err != nil {
		return errs.Wrap("cms.repository.upsert", errs.KindOf(err), err)
	}
	return nil
}

// toFields converts a SafetyIncident to the field map shape the CMS API
// expects. We pin the key/value names here so the use case stays decoupled
// from the wire format.
func toFields(s domain.SafetyIncident) map[string]any {
	return map[string]any{
		"key_cd":             s.KeyCd,
		"info_type":          s.InfoType,
		"info_name":          s.InfoName,
		"leave_date":         formatRFC3339(s.LeaveDate),
		"title":              s.Title,
		"lead":               s.Lead,
		"main_text":          s.MainText,
		"info_url":           s.InfoURL,
		"koukan_cd":          s.KoukanCd,
		"koukan_name":        s.KoukanName,
		"area_cd":            s.AreaCd,
		"area_name":          s.AreaName,
		"country_cd":         s.CountryCd,
		"country_name":       s.CountryName,
		"extracted_location": s.ExtractedLocation,
		"geometry":           geometryFor(s.Geometry),
		"geocode_source":     s.GeocodeSource.String(),
		"ingested_at":        formatRFC3339(s.IngestedAt),
		"updated_at":         formatRFC3339(s.UpdatedAt),
	}
}

// geometryFor encodes the Point as a GeoJSON-ish shape; the CMS field type
// is GeometryObject so reearth-cms accepts the GeoJSON form.
func geometryFor(p domain.Point) map[string]any {
	return map[string]any{
		"type":        "Point",
		"coordinates": []float64{p.Lng, p.Lat}, // GeoJSON is [lng, lat]
	}
}

// formatRFC3339 shapes timestamps as RFC3339 strings — matches the FieldType
// "date" wire format used by U-CSS.
func formatRFC3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// SizeForLog returns the field count for diagnostic logs (currently unused
// by callers, kept exported because it's the kind of thing operators ask
// for when triaging "is this writing all 19 fields?" questions).
func SizeForLog(s domain.SafetyIncident) string { return strconv.Itoa(len(toFields(s))) }
