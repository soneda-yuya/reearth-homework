package cms

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/soneda-yuya/overseas-safety-map/internal/platform/cmsx"
	"github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/domain"
	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
)

// ReaderClient is the subset of cmsx.Client the read-side adapter uses.
// Declared as an interface so tests can swap in a stub that does not
// spin up an httptest server.
type ReaderClient interface {
	ListItems(ctx context.Context, projectID, modelID string, q cmsx.ListItemsQuery) (cmsx.ListItemsResult, error)
	SearchItems(ctx context.Context, projectID, modelID string, q cmsx.ListItemsQuery) (cmsx.ListItemsResult, error)
	FindItemByFieldValue(ctx context.Context, projectID, modelID, fieldKey, value string) (*cmsx.ItemDTO, error)
}

// Reader fulfils domain.SafetyIncidentReader against reearth-cms. projectID,
// modelID and keyField are resolved by U-CSS at startup, same as Repository
// on the write side. projectID is required because reearth-cms's item paths
// nest under /{workspace}/projects/{project}/models/{model}/items.
type Reader struct {
	client    ReaderClient
	projectID string
	modelID   string
	keyField  string
	// nearbyFetchCap bounds the ListNearby broad fetch. Larger than the
	// caller's limit because the distance filter prunes most of the set;
	// smaller than the full corpus so a truly empty ListFilter does not
	// stall the RPC.
	nearbyFetchCap int
}

// NewReader returns a Reader wired to the same Model the Repository writes to.
func NewReader(client ReaderClient, projectID, modelID, keyField string) *Reader {
	return &Reader{
		client:         client,
		projectID:      projectID,
		modelID:        modelID,
		keyField:       keyField,
		nearbyFetchCap: 1000,
	}
}

// List returns a page of incidents matching filter. Cursor is passed straight
// to the CMS; the returned nextCursor is likewise opaque.
func (r *Reader) List(ctx context.Context, filter domain.ListFilter) ([]domain.SafetyIncident, string, error) {
	q := listQueryFromFilter(filter)
	res, err := r.client.ListItems(ctx, r.projectID, r.modelID, q)
	if err != nil {
		return nil, "", errs.Wrap("cms.reader.list", errs.KindOf(err), err)
	}
	items := make([]domain.SafetyIncident, 0, len(res.Items))
	for _, dto := range res.Items {
		incident, err := fromFields(dto.Fields)
		if err != nil {
			return nil, "", errs.Wrap("cms.reader.list", errs.KindOf(err), err)
		}
		items = append(items, incident)
	}
	return items, res.NextCursor, nil
}

// Get returns the incident for keyCd, or errs.KindNotFound if absent.
func (r *Reader) Get(ctx context.Context, keyCd string) (*domain.SafetyIncident, error) {
	if keyCd == "" {
		return nil, errs.Wrap("cms.reader.get", errs.KindInvalidInput,
			errors.New("key_cd is required"))
	}
	dto, err := r.client.FindItemByFieldValue(ctx, r.projectID, r.modelID, r.keyField, keyCd)
	if err != nil {
		return nil, errs.Wrap("cms.reader.get", errs.KindOf(err), err)
	}
	if dto == nil {
		return nil, errs.Wrap("cms.reader.get", errs.KindNotFound,
			fmt.Errorf("no item with key_cd=%q", keyCd))
	}
	incident, err := fromFields(dto.Fields)
	if err != nil {
		return nil, errs.Wrap("cms.reader.get", errs.KindOf(err), err)
	}
	return &incident, nil
}

// Search performs a keyword query over the incident corpus. Other filters
// in SearchFilter narrow the result set alongside the keyword (area, country,
// info_types, and the leave_date window).
func (r *Reader) Search(ctx context.Context, filter domain.SearchFilter) ([]domain.SafetyIncident, string, error) {
	q := cmsx.ListItemsQuery{
		Filters:   map[string]string{},
		InfoTypes: filter.InfoTypes,
		Keyword:   filter.Query,
		Limit:     filter.Limit,
		Cursor:    filter.Cursor,
	}
	if filter.AreaCd != "" {
		q.Filters["area_cd"] = filter.AreaCd
	}
	if filter.CountryCd != "" {
		q.Filters["country_cd"] = filter.CountryCd
	}
	if !filter.LeaveFrom.IsZero() {
		q.Filters["leave_from"] = filter.LeaveFrom.UTC().Format(time.RFC3339)
	}
	if !filter.LeaveTo.IsZero() {
		q.Filters["leave_to"] = filter.LeaveTo.UTC().Format(time.RFC3339)
	}
	res, err := r.client.SearchItems(ctx, r.projectID, r.modelID, q)
	if err != nil {
		return nil, "", errs.Wrap("cms.reader.search", errs.KindOf(err), err)
	}
	items := make([]domain.SafetyIncident, 0, len(res.Items))
	for _, dto := range res.Items {
		incident, err := fromFields(dto.Fields)
		if err != nil {
			return nil, "", errs.Wrap("cms.reader.search", errs.KindOf(err), err)
		}
		items = append(items, incident)
	}
	return items, res.NextCursor, nil
}

// ListNearby fetches a broad page (capped at nearbyFetchCap) and filters
// locally with the Haversine formula. The CMS does not currently support
// spatial queries; when it does, replace this with a server-side filter.
func (r *Reader) ListNearby(ctx context.Context, center domain.Point, radiusKm float64, limit int) ([]domain.SafetyIncident, error) {
	if radiusKm <= 0 || limit <= 0 {
		return nil, errs.Wrap("cms.reader.list_nearby", errs.KindInvalidInput,
			errors.New("radius_km and limit must be positive"))
	}
	res, err := r.client.ListItems(ctx, r.projectID, r.modelID, cmsx.ListItemsQuery{Limit: r.nearbyFetchCap})
	if err != nil {
		return nil, errs.Wrap("cms.reader.list_nearby", errs.KindOf(err), err)
	}

	type scored struct {
		item     domain.SafetyIncident
		distance float64
	}
	hits := make([]scored, 0, len(res.Items))
	for _, dto := range res.Items {
		incident, err := fromFields(dto.Fields)
		if err != nil {
			return nil, errs.Wrap("cms.reader.list_nearby", errs.KindOf(err), err)
		}
		d := haversineKm(center, incident.Geometry)
		if d <= radiusKm {
			hits = append(hits, scored{item: incident, distance: d})
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].distance < hits[j].distance })
	if len(hits) > limit {
		hits = hits[:limit]
	}
	out := make([]domain.SafetyIncident, len(hits))
	for i, h := range hits {
		out[i] = h.item
	}
	return out, nil
}

// listQueryFromFilter assembles the ListItemsQuery for a ListFilter. Kept
// separate so List and a future unified filter path can share the logic.
func listQueryFromFilter(f domain.ListFilter) cmsx.ListItemsQuery {
	q := cmsx.ListItemsQuery{
		Filters:   map[string]string{},
		InfoTypes: f.InfoTypes,
		Limit:     f.Limit,
		Cursor:    f.Cursor,
	}
	if f.AreaCd != "" {
		q.Filters["area_cd"] = f.AreaCd
	}
	if f.CountryCd != "" {
		q.Filters["country_cd"] = f.CountryCd
	}
	if !f.LeaveFrom.IsZero() {
		q.Filters["leave_from"] = f.LeaveFrom.UTC().Format(time.RFC3339)
	}
	if !f.LeaveTo.IsZero() {
		q.Filters["leave_to"] = f.LeaveTo.UTC().Format(time.RFC3339)
	}
	return q
}

// haversineKm returns the great-circle distance between two WGS84 points.
// Accurate enough for MVP proximity queries (≤ ~0.5% error).
func haversineKm(a, b domain.Point) float64 {
	const earthKm = 6371.0
	toRad := func(d float64) float64 { return d * math.Pi / 180 }
	lat1, lat2 := toRad(a.Lat), toRad(b.Lat)
	dLat := toRad(b.Lat - a.Lat)
	dLng := toRad(b.Lng - a.Lng)
	s := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLng/2)*math.Sin(dLng/2)
	c := 2 * math.Asin(math.Min(1, math.Sqrt(s)))
	return earthKm * c
}
