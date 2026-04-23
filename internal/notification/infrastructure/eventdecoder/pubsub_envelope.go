// Package eventdecoder parses Pub/Sub push HTTP envelopes into domain
// events. Keeping this in infrastructure means the application layer never
// sees base64 / Pub/Sub-specific JSON.
package eventdecoder

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/soneda-yuya/reearth-homework/internal/notification/domain"
	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

// PubSubEnvelopeDecoder decodes Pub/Sub push request bodies.
//
// Envelope shape (documented at
// https://cloud.google.com/pubsub/docs/push#receive_push):
//
//	{
//	  "message": {
//	    "data": "<base64 of NewArrivalEvent JSON>",
//	    "attributes": { "key_cd": "...", ... },
//	    "messageId": "..."
//	  },
//	  "subscription": "projects/.../subscriptions/..."
//	}
//
// The inner JSON is what U-ING publishes from
// internal/safetyincident/infrastructure/eventbus/publisher.go. Any parse
// failure is KindInvalidInput so the handler can map it to HTTP 400 and
// Pub/Sub will route the message straight to the DLQ (no retries).
type PubSubEnvelopeDecoder struct{}

// New returns a stateless decoder.
func New() PubSubEnvelopeDecoder { return PubSubEnvelopeDecoder{} }

// Decode parses the envelope → inner NewArrivalEvent.
func (PubSubEnvelopeDecoder) Decode(body []byte) (domain.NewArrivalEvent, error) {
	var env struct {
		Message struct {
			Data       string            `json:"data"`
			Attributes map[string]string `json:"attributes"`
		} `json:"message"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return domain.NewArrivalEvent{}, errs.Wrap("eventdecoder.envelope",
			errs.KindInvalidInput, err)
	}
	if env.Message.Data == "" {
		return domain.NewArrivalEvent{}, errs.Wrap("eventdecoder.empty_data",
			errs.KindInvalidInput, errInvalidEnvelope)
	}

	data, err := base64.StdEncoding.DecodeString(env.Message.Data)
	if err != nil {
		return domain.NewArrivalEvent{}, errs.Wrap("eventdecoder.base64",
			errs.KindInvalidInput, err)
	}

	var inner struct {
		KeyCd     string `json:"key_cd"`
		CountryCd string `json:"country_cd"`
		InfoType  string `json:"info_type"`
		Title     string `json:"title"`
		Lead      string `json:"lead"`
		LeaveDate string `json:"leave_date"` // RFC3339
		Geometry  struct {
			Lat float64 `json:"lat"`
			Lng float64 `json:"lng"`
		} `json:"geometry"`
	}
	if err := json.Unmarshal(data, &inner); err != nil {
		return domain.NewArrivalEvent{}, errs.Wrap("eventdecoder.inner_json",
			errs.KindInvalidInput, err)
	}

	var leaveDate time.Time
	if inner.LeaveDate != "" {
		t, err := time.Parse(time.RFC3339, inner.LeaveDate)
		if err != nil {
			return domain.NewArrivalEvent{}, errs.Wrap("eventdecoder.leave_date",
				errs.KindInvalidInput, err)
		}
		leaveDate = t
	}

	// Attributes may also carry key_cd / country_cd / info_type (U-ING
	// publisher writes them). Fall back to them when the inner body is
	// missing a value — message IDs should still be deduppable even if a
	// future publisher trims the body.
	if inner.KeyCd == "" {
		inner.KeyCd = env.Message.Attributes["key_cd"]
	}
	if inner.CountryCd == "" {
		inner.CountryCd = env.Message.Attributes["country_cd"]
	}
	if inner.InfoType == "" {
		inner.InfoType = env.Message.Attributes["info_type"]
	}

	if inner.KeyCd == "" {
		return domain.NewArrivalEvent{}, errs.Wrap("eventdecoder.missing_key_cd",
			errs.KindInvalidInput, errMissingKeyCd)
	}

	return domain.NewArrivalEvent{
		KeyCd:     inner.KeyCd,
		CountryCd: inner.CountryCd,
		InfoType:  inner.InfoType,
		Title:     inner.Title,
		Lead:      inner.Lead,
		LeaveDate: leaveDate,
	}, nil
}

// sentinel errors so tests can branch on them without tying to string match.
var (
	errInvalidEnvelope = stringErr("empty message.data")
	errMissingKeyCd    = stringErr("missing key_cd in message body and attributes")
)

type stringErr string

func (s stringErr) Error() string { return string(s) }
