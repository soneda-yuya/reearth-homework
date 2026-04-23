package domain

import "time"

// NewArrivalEvent is the domain view of the Pub/Sub message published by
// U-ING. It carries just enough to build a notification body and resolve
// subscribers; the authoritative SafetyIncident lives in reearth-cms.
type NewArrivalEvent struct {
	KeyCd     string
	CountryCd string
	InfoType  string
	Title     string
	Lead      string
	LeaveDate time.Time
}
