// Package mofa is the MOFA OpenData adapter. The XML shape declared here was
// reconciled against the live feed on 2026-04-24 (the hypothetical layout in
// U-ING design Q C [A] turned out to be wrong on several axes — see the
// field tags and nested area / country elements below).
package mofa

import (
	"encoding/xml"
	"time"
)

// mofaFeed is the document root. MOFA serves <opendata> with repeated <mail>
// children; each <mail> carries the per-incident fields plus nested <area>
// and <country> elements.
type mofaFeed struct {
	XMLName xml.Name  `xml:"opendata"`
	Items   []rawItem `xml:"mail"`
}

// rawItem mirrors the on-the-wire field names. Wire uses camelCase, not
// snake_case; area / country are nested elements; koukan* stay flat at the
// <mail> level. We keep LeaveDate as a string so parseLeaveDate can walk the
// format list without breaking the rest of the document.
type rawItem struct {
	KeyCd      string     `xml:"keyCd"`
	InfoType   string     `xml:"infoType"`
	InfoName   string     `xml:"infoName"`
	LeaveDate  string     `xml:"leaveDate"`
	Title      string     `xml:"title"`
	Lead       string     `xml:"lead"`
	MainText   string     `xml:"mainText"`
	InfoURL    string     `xml:"infoUrl"`
	KoukanCd   string     `xml:"koukanCd"`
	KoukanName string     `xml:"koukanName"`
	Area       *areaNode  `xml:"area"`
	Country    *countryND `xml:"country"`
}

// areaNode is the nested <area><cd>/<name></area> element. Only present when
// the mail is area-scoped; otherwise nil and the flat fields stay zero.
type areaNode struct {
	Cd   string `xml:"cd"`
	Name string `xml:"name"`
}

// countryND is the nested <country areaCd="..."><cd>/<name></country> element.
// areaCd on the XML attribute is redundant with <area><cd> when both are
// present, but keep it for future cross-reference if needed.
type countryND struct {
	AreaCd string `xml:"areaCd,attr"`
	Cd     string `xml:"cd"`
	Name   string `xml:"name"`
}

// flatAreaCd returns the inner <area><cd> text or empty. Callers use this to
// fill the flattened SafetyIncident fields.
func (r rawItem) flatAreaCd() string {
	if r.Area == nil {
		return ""
	}
	return r.Area.Cd
}

func (r rawItem) flatAreaName() string {
	if r.Area == nil {
		return ""
	}
	return r.Area.Name
}

func (r rawItem) flatCountryCd() string {
	if r.Country == nil {
		return ""
	}
	return r.Country.Cd
}

func (r rawItem) flatCountryName() string {
	if r.Country == nil {
		return ""
	}
	return r.Country.Name
}

// leaveDateFormats lists the timestamps we accept on the wire, in order of
// preference. The first that parses wins; if none match the row is dropped
// with a warning at the application layer. The Japanese-style
// "YYYY/MM/DD HH:MM:SS" matches the live MOFA feed; the others are kept as a
// safety net in case MOFA reformats.
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
