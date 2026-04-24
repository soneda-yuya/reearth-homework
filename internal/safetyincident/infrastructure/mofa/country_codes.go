package mofa

import (
	_ "embed"
	"encoding/json"
	"sync"
)

//go:embed country_codes.json
var countryCodesRaw []byte

var (
	countryCodesOnce sync.Once
	countryCodes     map[string]string
	countryCodesErr  error
)

// MOFACodeToISO returns the ISO 3166-1 alpha-2 code corresponding to the
// MOFA numeric country code (e.g. "0049" → "DE"). An unknown code returns
// "". The mapping is loaded once on first call.
//
// MOFA's own country.xlsx is the source; we embed it as JSON because the
// list is stable (countries don't get added every month) and keeping the
// lookup in-process avoids a Cloud Run cold-start penalty.
func MOFACodeToISO(code string) string {
	countryCodesOnce.Do(func() {
		countryCodesErr = json.Unmarshal(countryCodesRaw, &countryCodes)
	})
	if countryCodesErr != nil {
		return ""
	}
	return countryCodes[code]
}
