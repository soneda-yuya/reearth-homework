// Package domain holds the safety-incident bounded context aggregates and
// value objects used by the ingestion / BFF / notifier units.
package domain

import (
	"errors"
	"fmt"
	"time"

	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
)

// MailItem is the unprocessed MOFA item — what comes out of the XML parser
// before LLM extraction or geocoding. Field names mirror the proto schema
// (snake_case → CamelCase) to keep downstream conversions trivial.
type MailItem struct {
	KeyCd       string
	InfoType    string
	InfoName    string
	LeaveDate   time.Time
	Title       string
	Lead        string
	MainText    string
	InfoURL     string
	KoukanCd    string
	KoukanName  string
	AreaCd      string
	AreaName    string
	CountryCd   string
	CountryName string
}

// Validate enforces the minimum invariants we need before the item can flow
// through the use case. The remaining fields are best-effort: MOFA sometimes
// publishes items with empty info_name or area_name.
//
// country_cd is intentionally NOT required at this layer: MOFA occasionally
// publishes items without a nested <country> element (global advisories,
// sample/template entries). The geocoder chain backfills the field from
// Mapbox when a specific location can be resolved; items that fail even
// that fallback are dropped later in the pipeline, not here.
func (m MailItem) Validate() error {
	var violations []error
	if m.KeyCd == "" {
		violations = append(violations, errors.New("key_cd is required"))
	}
	if m.LeaveDate.IsZero() {
		violations = append(violations, errors.New("leave_date is required"))
	}
	if m.Title == "" {
		violations = append(violations, errors.New("title is required"))
	}
	if len(violations) == 0 {
		return nil
	}
	return errs.Wrap(
		fmt.Sprintf("mail_item.validate(key_cd=%q)", m.KeyCd),
		errs.KindInvalidInput,
		errors.Join(violations...),
	)
}
