package rpc

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	overseasmapv1 "github.com/soneda-yuya/overseas-safety-map/gen/go/v1"
	crimemap "github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/crimemap/domain"
	safetyincident "github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/domain"
	userdom "github.com/soneda-yuya/overseas-safety-map/internal/user/domain"
)

// incidentToProto converts a domain incident to the wire shape. Timestamps
// use timestamppb — Go time.Time zero values become nil so the client can
// distinguish "not set" from "epoch".
func incidentToProto(s safetyincident.SafetyIncident) *overseasmapv1.SafetyIncident {
	return &overseasmapv1.SafetyIncident{
		KeyCd:             s.KeyCd,
		InfoType:          s.InfoType,
		InfoName:          s.InfoName,
		LeaveDate:         tsOrNil(s.LeaveDate),
		Title:             s.Title,
		Lead:              s.Lead,
		MainText:          s.MainText,
		InfoUrl:           s.InfoURL,
		KoukanCd:          s.KoukanCd,
		KoukanName:        s.KoukanName,
		AreaCd:            s.AreaCd,
		AreaName:          s.AreaName,
		CountryCd:         s.CountryCd,
		CountryName:       s.CountryName,
		ExtractedLocation: s.ExtractedLocation,
		Geometry:          pointToProto(s.Geometry),
		GeocodeSource:     geocodeSourceToProto(s.GeocodeSource),
		IngestedAt:        tsOrNil(s.IngestedAt),
		UpdatedAt:         tsOrNil(s.UpdatedAt),
	}
}

func pointToProto(p safetyincident.Point) *overseasmapv1.Point {
	return &overseasmapv1.Point{Lat: p.Lat, Lng: p.Lng}
}

func pointFromProto(p *overseasmapv1.Point) safetyincident.Point {
	if p == nil {
		return safetyincident.Point{}
	}
	return safetyincident.Point{Lat: p.GetLat(), Lng: p.GetLng()}
}

func geocodeSourceToProto(s safetyincident.GeocodeSource) overseasmapv1.GeocodeSource {
	switch s {
	case safetyincident.GeocodeSourceMapbox:
		return overseasmapv1.GeocodeSource_GEOCODE_SOURCE_MAPBOX
	case safetyincident.GeocodeSourceCountryCentroid:
		return overseasmapv1.GeocodeSource_GEOCODE_SOURCE_COUNTRY_CENTROID
	default:
		return overseasmapv1.GeocodeSource_GEOCODE_SOURCE_UNSPECIFIED
	}
}

func listFilterFromProto(f *overseasmapv1.SafetyIncidentFilter) safetyincident.ListFilter {
	if f == nil {
		return safetyincident.ListFilter{}
	}
	// A nil *timestamppb.Timestamp surfaces through .AsTime() as the Unix
	// epoch (1970-01-01), NOT time.Time{}. Downstream callers use IsZero()
	// to decide whether to apply the leave window; bypassing that via a
	// nil check keeps "no leave filter" semantics intact.
	var leaveFrom, leaveTo time.Time
	if ts := f.GetLeaveFrom(); ts != nil {
		leaveFrom = ts.AsTime()
	}
	if ts := f.GetLeaveTo(); ts != nil {
		leaveTo = ts.AsTime()
	}
	return safetyincident.ListFilter{
		AreaCd:    f.GetAreaCd(),
		CountryCd: f.GetCountryCd(),
		InfoTypes: f.GetInfoTypes(),
		LeaveFrom: leaveFrom,
		LeaveTo:   leaveTo,
		Limit:     int(f.GetLimit()),
		Cursor:    f.GetCursor(),
	}
}

// searchFilterFromProto reuses SafetyIncidentFilter plus the RPC's separate
// query argument; the handler passes the query in explicitly.
func searchFilterFromProto(f *overseasmapv1.SafetyIncidentFilter, query string) safetyincident.SearchFilter {
	base := listFilterFromProto(f)
	return safetyincident.SearchFilter{
		Query:     query,
		AreaCd:    base.AreaCd,
		CountryCd: base.CountryCd,
		InfoTypes: base.InfoTypes,
		LeaveFrom: base.LeaveFrom,
		LeaveTo:   base.LeaveTo,
		Limit:     base.Limit,
		Cursor:    base.Cursor,
	}
}

func crimeMapFilterFromProto(f *overseasmapv1.CrimeMapFilter) crimemap.CrimeMapFilter {
	if f == nil {
		return crimemap.CrimeMapFilter{}
	}
	return crimemap.CrimeMapFilter{
		LeaveFrom: f.GetLeaveFrom().AsTime(),
		LeaveTo:   f.GetLeaveTo().AsTime(),
	}
}

func choroplethToProto(res crimemap.ChoroplethResult) []*overseasmapv1.CountryChoropleth {
	out := make([]*overseasmapv1.CountryChoropleth, 0, len(res.Items))
	for _, it := range res.Items {
		out = append(out, &overseasmapv1.CountryChoropleth{
			CountryCd:   it.CountryCd,
			CountryName: it.CountryName,
			Count:       int32(it.Count),
			Color:       it.Color,
		})
	}
	return out
}

func heatmapToProto(res crimemap.HeatmapResult) []*overseasmapv1.HeatmapPoint {
	out := make([]*overseasmapv1.HeatmapPoint, 0, len(res.Points))
	for _, p := range res.Points {
		out = append(out, &overseasmapv1.HeatmapPoint{
			Location: &overseasmapv1.Point{Lat: p.Location.Lat, Lng: p.Location.Lng},
			Weight:   p.Weight,
		})
	}
	return out
}

func userProfileToProto(p *userdom.UserProfile) *overseasmapv1.UserProfile {
	if p == nil {
		return nil
	}
	return &overseasmapv1.UserProfile{
		Uid:                p.UID,
		FavoriteCountryCds: p.FavoriteCountryCds,
		NotificationPreference: &overseasmapv1.NotificationPreference{
			Enabled:          p.NotificationPreference.Enabled,
			TargetCountryCds: p.NotificationPreference.TargetCountryCds,
			InfoTypes:        p.NotificationPreference.InfoTypes,
		},
		FcmTokenCount: int32(len(p.FCMTokens)),
	}
}

func notificationPrefFromProto(p *overseasmapv1.NotificationPreference) userdom.NotificationPreference {
	if p == nil {
		return userdom.NotificationPreference{}
	}
	return userdom.NotificationPreference{
		Enabled:          p.GetEnabled(),
		TargetCountryCds: p.GetTargetCountryCds(),
		InfoTypes:        p.GetInfoTypes(),
	}
}

// tsOrNil returns a proto Timestamp, or nil when t is the zero time. The
// nil-for-zero contract lets clients distinguish "unset" from "epoch".
func tsOrNil(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}
