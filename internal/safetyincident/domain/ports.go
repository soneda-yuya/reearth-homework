package domain

import (
	"context"
	"time"
)

// IngestionMode selects the MOFA endpoint the source should query.
//
//	IngestionModeIncremental → /newarrivalA.xml (5-minute polling default)
//	IngestionModeInitial     → /00A.xml         (manual one-shot backfill)
type IngestionMode string

const (
	IngestionModeIncremental IngestionMode = "incremental"
	IngestionModeInitial     IngestionMode = "initial"
)

// MofaSource pulls items from MOFA. Implementations are responsible for the
// transport details (HTTP retry, XML parsing) and surface a homogeneous
// []MailItem regardless of mode.
type MofaSource interface {
	Fetch(ctx context.Context, mode IngestionMode) ([]MailItem, error)
}

// LocationExtractor turns one MailItem into a (location, confidence) pair.
// The implementation is LLM-backed today; see U-ING design Q B.
type LocationExtractor interface {
	Extract(ctx context.Context, item MailItem) (ExtractResult, error)
}

// ExtractResult is what LocationExtractor returns. An empty Location with
// Confidence=0 is the documented "I can't tell" signal — Geocoder treats it
// as a centroid-only path rather than as an error.
type ExtractResult struct {
	Location   string
	Confidence float64
}

// Geocoder turns (location, country) into a coordinate. ChainGeocoder is the
// production implementation: Mapbox first, country centroid as fallback.
type Geocoder interface {
	Geocode(ctx context.Context, location, countryCd string) (GeocodeResult, error)
}

// GeocodeResult carries both the resolved Point and the chain stage that
// produced it. Callers persist Source so downstream UI can label "approximate
// (country level)" pins.
//
// CountryCd, when non-empty, is the ISO 3166-1 alpha-2 code the geocoder
// derived from the location. Ingestion uses this to backfill MailItem.CountryCd
// on items where MOFA shipped no <country> element, so the resulting
// SafetyIncident still carries a usable country for filters / flags.
type GeocodeResult struct {
	Point     Point
	Source    GeocodeSource
	CountryCd string
}

// Repository is the CMS persistence port. Exists is split out from Upsert so
// the use case can short-circuit LLM/Mapbox calls when the item is already
// known (idempotency by CMS lookup, U-ING design Q3 [A]).
type Repository interface {
	Exists(ctx context.Context, keyCd string) (bool, error)
	Upsert(ctx context.Context, incident SafetyIncident) error
}

// EventPublisher fans new arrivals out to the notification context via
// Pub/Sub. Failure here is non-fatal — the CMS already has the item; the
// publish loss is logged so an operator can rebroadcast manually.
type EventPublisher interface {
	PublishNewArrival(ctx context.Context, ev NewArrivalEvent) error
}

// NewArrivalEvent is the payload Pub/Sub carries to the notifier. It is a
// thin DTO — full SafetyIncident details are not duplicated here so the
// notifier can re-fetch by KeyCd if it wants the latest shape.
type NewArrivalEvent struct {
	KeyCd     string
	CountryCd string
	InfoType  string
	Geometry  Point
	LeaveDate time.Time
}
