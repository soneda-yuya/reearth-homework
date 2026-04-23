package rpc

import (
	"context"
	"encoding/json"

	"connectrpc.com/connect"

	overseasmapv1 "github.com/soneda-yuya/overseas-safety-map/gen/go/v1"
	"github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/application"
	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
)

// SafetyIncidentServer implements SafetyIncidentServiceHandler against the
// five read-side use cases. All of them receive the request context unchanged
// — authorization has already happened in AuthInterceptor.
type SafetyIncidentServer struct {
	list    *application.ListUseCase
	get     *application.GetUseCase
	search  *application.SearchUseCase
	nearby  *application.NearbyUseCase
	geoJSON *application.GeoJSONUseCase
}

// NewSafetyIncidentServer wires the use case dependencies.
func NewSafetyIncidentServer(
	list *application.ListUseCase,
	get *application.GetUseCase,
	search *application.SearchUseCase,
	nearby *application.NearbyUseCase,
	geoJSON *application.GeoJSONUseCase,
) *SafetyIncidentServer {
	return &SafetyIncidentServer{list: list, get: get, search: search, nearby: nearby, geoJSON: geoJSON}
}

// ListSafetyIncidents returns a page of incidents.
func (s *SafetyIncidentServer) ListSafetyIncidents(
	ctx context.Context,
	req *connect.Request[overseasmapv1.ListSafetyIncidentsRequest],
) (*connect.Response[overseasmapv1.ListSafetyIncidentsResponse], error) {
	items, next, err := s.list.Execute(ctx, listFilterFromProto(req.Msg.GetFilter()))
	if err != nil {
		return nil, err
	}
	out := make([]*overseasmapv1.SafetyIncident, 0, len(items))
	for i := range items {
		out = append(out, incidentToProto(items[i]))
	}
	return connect.NewResponse(&overseasmapv1.ListSafetyIncidentsResponse{
		Items:      out,
		NextCursor: next,
		TotalHint:  int32(len(items)),
	}), nil
}

// GetSafetyIncident returns a single incident by key_cd.
func (s *SafetyIncidentServer) GetSafetyIncident(
	ctx context.Context,
	req *connect.Request[overseasmapv1.GetSafetyIncidentRequest],
) (*connect.Response[overseasmapv1.GetSafetyIncidentResponse], error) {
	item, err := s.get.Execute(ctx, req.Msg.GetKeyCd())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&overseasmapv1.GetSafetyIncidentResponse{
		Item: incidentToProto(*item),
	}), nil
}

// SearchSafetyIncidents returns a page of keyword-matching incidents. The
// RPC currently reuses SafetyIncidentFilter as the filter container with the
// query carried on cursor for now; until a dedicated query field is added to
// proto we pull it from Filter.Cursor only when it starts with "q=".
// TODO: add `string query` to SearchSafetyIncidentsRequest in proto.
func (s *SafetyIncidentServer) SearchSafetyIncidents(
	ctx context.Context,
	req *connect.Request[overseasmapv1.SearchSafetyIncidentsRequest],
) (*connect.Response[overseasmapv1.SearchSafetyIncidentsResponse], error) {
	filter := req.Msg.GetFilter()
	// SearchSafetyIncidentsRequest has no explicit query field yet (proto
	// reuses SafetyIncidentFilter). Use cursor for now as a keyword carrier
	// so the handler is wired end-to-end; the proto will get a real field.
	query := filter.GetCursor()
	items, next, err := s.search.Execute(ctx, searchFilterFromProto(filter, query))
	if err != nil {
		return nil, err
	}
	out := make([]*overseasmapv1.SafetyIncident, 0, len(items))
	for i := range items {
		out = append(out, incidentToProto(items[i]))
	}
	return connect.NewResponse(&overseasmapv1.SearchSafetyIncidentsResponse{
		Items:      out,
		NextCursor: next,
	}), nil
}

// ListNearby returns the top-N incidents within radius_km of center.
func (s *SafetyIncidentServer) ListNearby(
	ctx context.Context,
	req *connect.Request[overseasmapv1.ListNearbyRequest],
) (*connect.Response[overseasmapv1.ListNearbyResponse], error) {
	items, err := s.nearby.Execute(ctx,
		pointFromProto(req.Msg.GetCenter()),
		req.Msg.GetRadiusKm(),
		int(req.Msg.GetLimit()))
	if err != nil {
		return nil, err
	}
	out := make([]*overseasmapv1.SafetyIncident, 0, len(items))
	for i := range items {
		out = append(out, incidentToProto(items[i]))
	}
	return connect.NewResponse(&overseasmapv1.ListNearbyResponse{Items: out}), nil
}

// GetSafetyIncidentsAsGeoJSON returns the result as a JSON blob.
func (s *SafetyIncidentServer) GetSafetyIncidentsAsGeoJSON(
	ctx context.Context,
	req *connect.Request[overseasmapv1.GetSafetyIncidentsAsGeoJSONRequest],
) (*connect.Response[overseasmapv1.GetSafetyIncidentsAsGeoJSONResponse], error) {
	fc, err := s.geoJSON.Execute(ctx, listFilterFromProto(req.Msg.GetFilter()))
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(fc)
	if err != nil {
		return nil, errs.Wrap("rpc.geojson.marshal", errs.KindInternal, err)
	}
	return connect.NewResponse(&overseasmapv1.GetSafetyIncidentsAsGeoJSONResponse{
		Geojson: raw,
	}), nil
}
