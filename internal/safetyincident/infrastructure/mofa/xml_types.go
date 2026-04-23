// Package mofa is the MOFA OpenData adapter. The XML shape declared here is
// a *plausible* layout for the Foreign Ministry's open data feed; the real
// element names and casing are confirmed during U-ING Build and Test (see
// U-ING design Q C [A] — implement against an assumption, reconcile with the
// live feed when actually exercising the binary).
package mofa

import (
	"encoding/xml"
	"time"
)

// mofaFeed is the document root. The <items> wrapper plus repeated <item>
// elements is the standard shape for both endpoints (00A.xml / newarrivalA.xml).
type mofaFeed struct {
	XMLName xml.Name  `xml:"items"`
	Items   []rawItem `xml:"item"`
}

// rawItem mirrors the on-the-wire field names. snake_case → CamelCase Go
// fields, with xml tags pointing back at the wire form. We deliberately keep
// LeaveDate as a string here so a custom parse (RFC3339 vs Japanese-format)
// can decode it without breaking the rest of the document.
type rawItem struct {
	KeyCd       string `xml:"key_cd"`
	InfoType    string `xml:"info_type"`
	InfoName    string `xml:"info_name"`
	LeaveDate   string `xml:"leave_date"`
	Title       string `xml:"title"`
	Lead        string `xml:"lead"`
	MainText    string `xml:"main_text"`
	InfoURL     string `xml:"info_url"`
	KoukanCd    string `xml:"koukan_cd"`
	KoukanName  string `xml:"koukan_name"`
	AreaCd      string `xml:"area_cd"`
	AreaName    string `xml:"area_name"`
	CountryCd   string `xml:"country_cd"`
	CountryName string `xml:"country_name"`
}

// leaveDateFormats lists the timestamps we accept on the wire, in order of
// preference. The first that parses wins; if none match the row is dropped
// with a warning at the application layer.
var leaveDateFormats = []string{
	time.RFC3339,
	"2006-01-02T15:04:05Z",
	"2006-01-02 15:04:05",
	"2006/01/02 15:04:05",
	"2006-01-02",
}

func parseLeaveDate(raw string) (time.Time, bool) {
	for _, f := range leaveDateFormats {
		if t, err := time.Parse(f, raw); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}
