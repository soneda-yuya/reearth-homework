package cms

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/domain"
	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
)

// fromFields is the inverse of toFields: converts the CMS Item.Fields map
// back into a domain.SafetyIncident. Missing optional fields are tolerated
// (MOFA frequently publishes items with empty info_name); missing required
// fields (key_cd, leave_date, title, geometry) surface as KindInternal
// because they indicate schema drift, not a user bug.
//
// country_cd is *not* required symmetrically with domain.MailItem.Validate:
// the ingestion pipeline persists items without a country when neither
// MOFA's <country> nor Mapbox's context surfaced one (rare but observed).
// Treating it as required here would make the whole List RPC 500 on a
// single missing-country item in the page. Downstream UI filters on
// country_cd already tolerate the empty string.
func fromFields(f map[string]any) (domain.SafetyIncident, error) {
	if f == nil {
		return domain.SafetyIncident{}, errs.Wrap("cms.from_fields",
			errs.KindInternal, errors.New("fields is nil"))
	}

	keyCd, err := requireString(f, "key_cd")
	if err != nil {
		return domain.SafetyIncident{}, err
	}
	title, err := requireString(f, "title")
	if err != nil {
		return domain.SafetyIncident{}, err
	}
	countryCd := optionalString(f, "country_cd")
	leaveDate, err := requireTime(f, "leave_date")
	if err != nil {
		return domain.SafetyIncident{}, err
	}
	point, err := parseGeometry(f["geometry"])
	if err != nil {
		return domain.SafetyIncident{}, err
	}
	ingestedAt, err := optionalTime(f, "ingested_at")
	if err != nil {
		return domain.SafetyIncident{}, err
	}
	updatedAt, err := optionalTime(f, "updated_at")
	if err != nil {
		return domain.SafetyIncident{}, err
	}

	return domain.SafetyIncident{
		MailItem: domain.MailItem{
			KeyCd:       keyCd,
			InfoType:    optionalString(f, "info_type"),
			InfoName:    optionalString(f, "info_name"),
			LeaveDate:   leaveDate,
			Title:       title,
			Lead:        optionalString(f, "lead"),
			MainText:    optionalString(f, "main_text"),
			InfoURL:     optionalString(f, "info_url"),
			KoukanCd:    optionalString(f, "koukan_cd"),
			KoukanName:  optionalString(f, "koukan_name"),
			AreaCd:      optionalString(f, "area_cd"),
			AreaName:    optionalString(f, "area_name"),
			CountryCd:   countryCd,
			CountryName: optionalString(f, "country_name"),
		},
		ExtractedLocation: optionalString(f, "extracted_location"),
		Geometry:          point,
		GeocodeSource:     parseGeocodeSource(optionalString(f, "geocode_source")),
		IngestedAt:        ingestedAt,
		UpdatedAt:         updatedAt,
	}, nil
}

func requireString(f map[string]any, key string) (string, error) {
	v, ok := f[key].(string)
	if !ok || v == "" {
		return "", errs.Wrap("cms.from_fields", errs.KindInternal,
			fmt.Errorf("missing required field %q", key))
	}
	return v, nil
}

func optionalString(f map[string]any, key string) string {
	if v, ok := f[key].(string); ok {
		return v
	}
	return ""
}

func requireTime(f map[string]any, key string) (time.Time, error) {
	raw, ok := f[key].(string)
	if !ok || raw == "" {
		return time.Time{}, errs.Wrap("cms.from_fields", errs.KindInternal,
			fmt.Errorf("missing required time field %q", key))
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, errs.Wrap("cms.from_fields", errs.KindInternal, err)
	}
	return t, nil
}

// optionalTime parses key as RFC3339 when present. Absent / empty values yield
// the zero time with no error (the field is optional). A present-but-malformed
// value returns KindInternal so CMS schema drift surfaces instead of silently
// producing a zero timestamp that downstream code would treat as "never set".
func optionalTime(f map[string]any, key string) (time.Time, error) {
	raw, ok := f[key].(string)
	if !ok || raw == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, errs.Wrap("cms.from_fields", errs.KindInternal,
			fmt.Errorf("field %q: invalid RFC3339: %w", key, err))
	}
	return t, nil
}

// parseGeometry decodes the GeoJSON-ish {type: "Point", coordinates: [lng, lat]}
// shape produced by toFields. Returns a zero Point with KindInternal when
// the shape is unrecognisable — we persisted it, so we must decode it.
//
// reearth-cms round-trips geometryObject fields as a JSON-stringified
// object on read (even though we POST a real object), so we accept both
// forms: pass a map[string]any through directly, or json.Unmarshal when the
// server handed us back a string.
func parseGeometry(v any) (domain.Point, error) {
	var obj map[string]any
	switch t := v.(type) {
	case map[string]any:
		obj = t
	case string:
		if t == "" {
			return domain.Point{}, errs.Wrap("cms.from_fields.geometry",
				errs.KindInternal, fmt.Errorf("geometry is empty string"))
		}
		if err := json.Unmarshal([]byte(t), &obj); err != nil {
			return domain.Point{}, errs.Wrap("cms.from_fields.geometry",
				errs.KindInternal, fmt.Errorf("geometry string is not JSON: %w", err))
		}
	default:
		return domain.Point{}, errs.Wrap("cms.from_fields.geometry",
			errs.KindInternal, fmt.Errorf("geometry is not an object (got %T)", v))
	}
	if obj == nil {
		return domain.Point{}, errs.Wrap("cms.from_fields.geometry",
			errs.KindInternal, fmt.Errorf("geometry object is nil"))
	}
	coordsRaw, ok := obj["coordinates"].([]any)
	if !ok || len(coordsRaw) < 2 {
		return domain.Point{}, errs.Wrap("cms.from_fields.geometry",
			errs.KindInternal, fmt.Errorf("geometry.coordinates missing or short"))
	}
	lng, ok1 := toFloat(coordsRaw[0])
	lat, ok2 := toFloat(coordsRaw[1])
	if !ok1 || !ok2 {
		return domain.Point{}, errs.Wrap("cms.from_fields.geometry",
			errs.KindInternal, fmt.Errorf("geometry.coordinates not numeric"))
	}
	return domain.Point{Lng: lng, Lat: lat}, nil
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	}
	return 0, false
}

func parseGeocodeSource(s string) domain.GeocodeSource {
	// Mirror GeocodeSource.String() in internal/safetyincident/domain.
	switch s {
	case "mapbox":
		return domain.GeocodeSourceMapbox
	case "country_centroid":
		return domain.GeocodeSourceCountryCentroid
	}
	return domain.GeocodeSourceUnspecified
}
