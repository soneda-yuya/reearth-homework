package domain

import (
	"context"
	"time"
)

// SafetyIncidentReader is the read-only port used by the BFF unit to fetch
// incidents from the storage backend (reearth-cms today, potentially BigQuery
// tomorrow). U-ING owns the write side; this interface is deliberately
// separate so read/write dependencies can evolve independently.
type SafetyIncidentReader interface {
	// List returns a page of incidents plus an opaque nextCursor. nextCursor
	// is empty when the caller has reached the end. filter.Cursor carries the
	// opaque token produced by the previous call.
	List(ctx context.Context, filter ListFilter) (items []SafetyIncident, nextCursor string, err error)
	// Get returns a single incident by its MOFA key_cd. Absent rows surface as
	// errs.KindNotFound so the RPC layer can translate to connect.CodeNotFound.
	Get(ctx context.Context, keyCd string) (*SafetyIncident, error)
	// Search performs a keyword lookup across the incident corpus and returns
	// the same (items, nextCursor) shape as List.
	Search(ctx context.Context, filter SearchFilter) (items []SafetyIncident, nextCursor string, err error)
	// ListNearby is a top-N proximity query; the caller bounds the result set
	// with `limit`, so pagination is not modeled at this layer. If a future
	// client needs paging, add a cursor return alongside `limit`.
	ListNearby(ctx context.Context, center Point, radiusKm float64, limit int) ([]SafetyIncident, error)
}

// ListFilter scopes a List call. Zero values mean "no constraint" on that
// field. Cursor is opaque (supplied by a previous page's nextCursor), empty
// means "first page".
type ListFilter struct {
	AreaCd    string
	CountryCd string
	InfoTypes []string
	LeaveFrom time.Time
	LeaveTo   time.Time
	Limit     int
	Cursor    string
}

// SearchFilter scopes a Search call. Query is the user-supplied keyword; the
// rest mirror ListFilter so search results can be narrowed the same way.
type SearchFilter struct {
	Query     string
	AreaCd    string
	CountryCd string
	InfoTypes []string
	LeaveFrom time.Time
	LeaveTo   time.Time
	Limit     int
	Cursor    string
}
