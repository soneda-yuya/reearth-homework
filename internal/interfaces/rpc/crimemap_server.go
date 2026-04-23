package rpc

import (
	"context"

	"connectrpc.com/connect"

	overseasmapv1 "github.com/soneda-yuya/overseas-safety-map/gen/go/v1"
	"github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/crimemap/application"
)

// CrimeMapServer implements CrimeMapServiceHandler using the Aggregator.
type CrimeMapServer struct {
	aggregator *application.Aggregator
}

// NewCrimeMapServer wires the aggregator.
func NewCrimeMapServer(aggregator *application.Aggregator) *CrimeMapServer {
	return &CrimeMapServer{aggregator: aggregator}
}

// GetChoropleth returns per-country counts with pre-computed colours.
func (s *CrimeMapServer) GetChoropleth(
	ctx context.Context,
	req *connect.Request[overseasmapv1.GetChoroplethRequest],
) (*connect.Response[overseasmapv1.GetChoroplethResponse], error) {
	res, err := s.aggregator.Choropleth(ctx, crimeMapFilterFromProto(req.Msg.GetFilter()))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&overseasmapv1.GetChoroplethResponse{
		Items: choroplethToProto(res),
		Total: int32(res.Total),
	}), nil
}

// GetHeatmap returns the raw point cloud plus excluded-fallback diagnostics.
func (s *CrimeMapServer) GetHeatmap(
	ctx context.Context,
	req *connect.Request[overseasmapv1.GetHeatmapRequest],
) (*connect.Response[overseasmapv1.GetHeatmapResponse], error) {
	res, err := s.aggregator.Heatmap(ctx, crimeMapFilterFromProto(req.Msg.GetFilter()))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&overseasmapv1.GetHeatmapResponse{
		Points:           heatmapToProto(res),
		ExcludedFallback: int32(res.ExcludedFallback),
	}), nil
}
