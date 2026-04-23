// Package eventbus is the safetyincident Pub/Sub adapter. It wraps a thin
// Topic abstraction so the use case can publish NewArrivalEvent without
// importing the cloud.google.com/go/pubsub SDK directly.
package eventbus

import (
	"context"
	"encoding/json"
	"time"

	"github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/domain"
	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
)

// Topic is what the EventPublisher needs from a Pub/Sub topic. The concrete
// wiring (against cloud.google.com/go/pubsub.Topic) lives in the
// composition root so this package stays SDK-free.
type Topic interface {
	// Publish enqueues a message and returns the server-assigned id once
	// the broker acknowledges it. data is the encoded payload; attributes
	// are surfaced as Pub/Sub message attributes for filtering.
	Publish(ctx context.Context, data []byte, attributes map[string]string) (string, error)
}

// Publisher fulfils domain.EventPublisher by JSON-encoding NewArrivalEvent
// onto a single Pub/Sub topic.
type Publisher struct {
	topic Topic
}

// New wires a Publisher to a Topic.
func New(topic Topic) *Publisher {
	return &Publisher{topic: topic}
}

// PublishNewArrival serialises ev and sends it. The KeyCd / CountryCd /
// InfoType become Pub/Sub attributes so the U-NTF subscriber can filter
// without having to decode the body.
func (p *Publisher) PublishNewArrival(ctx context.Context, ev domain.NewArrivalEvent) error {
	body, err := json.Marshal(newArrivalEventWire{
		KeyCd:     ev.KeyCd,
		CountryCd: ev.CountryCd,
		InfoType:  ev.InfoType,
		Geometry:  geometryWire{Lat: ev.Geometry.Lat, Lng: ev.Geometry.Lng},
		LeaveDate: ev.LeaveDate.UTC().Format(time.RFC3339),
	})
	if err != nil {
		return errs.Wrap("eventbus.marshal", errs.KindInternal, err)
	}

	attrs := map[string]string{
		"key_cd":     ev.KeyCd,
		"country_cd": ev.CountryCd,
		"info_type":  ev.InfoType,
	}
	if _, err := p.topic.Publish(ctx, body, attrs); err != nil {
		return errs.Wrap("eventbus.publish", errs.KindOf(err), err)
	}
	return nil
}

// newArrivalEventWire pins the JSON shape Pub/Sub messages carry. We avoid
// importing the proto-generated type here so the wire format doesn't bind
// to proto regeneration cadence — JSON is the lingua franca for Pub/Sub
// payloads in this codebase.
type newArrivalEventWire struct {
	KeyCd     string       `json:"key_cd"`
	CountryCd string       `json:"country_cd"`
	InfoType  string       `json:"info_type"`
	Geometry  geometryWire `json:"geometry"`
	LeaveDate string       `json:"leave_date"`
}

type geometryWire struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}
