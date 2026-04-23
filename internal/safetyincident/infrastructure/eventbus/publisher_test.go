package eventbus_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/soneda-yuya/reearth-homework/internal/safetyincident/domain"
	"github.com/soneda-yuya/reearth-homework/internal/safetyincident/infrastructure/eventbus"
)

type stubTopic struct {
	lastData       []byte
	lastAttributes map[string]string
	id             string
	err            error
}

func (s *stubTopic) Publish(_ context.Context, data []byte, attrs map[string]string) (string, error) {
	s.lastData = data
	s.lastAttributes = attrs
	if s.err != nil {
		return "", s.err
	}
	return s.id, nil
}

func sampleEvent() domain.NewArrivalEvent {
	return domain.NewArrivalEvent{
		KeyCd:     "k-1",
		CountryCd: "JP",
		InfoType:  "spot",
		Geometry:  domain.Point{Lat: 35.6, Lng: 139.7},
		LeaveDate: time.Date(2026, 4, 23, 9, 0, 0, 0, time.UTC),
	}
}

func TestPublisher_PublishNewArrival_HappyPath(t *testing.T) {
	t.Parallel()
	stub := &stubTopic{id: "msg-id-1"}
	pub := eventbus.New(stub)

	if err := pub.PublishNewArrival(context.Background(), sampleEvent()); err != nil {
		t.Fatalf("PublishNewArrival: %v", err)
	}

	// Attributes are exposed for subscriber-side filtering.
	if got := stub.lastAttributes["key_cd"]; got != "k-1" {
		t.Errorf("attr key_cd = %q", got)
	}
	if got := stub.lastAttributes["country_cd"]; got != "JP" {
		t.Errorf("attr country_cd = %q", got)
	}
	if got := stub.lastAttributes["info_type"]; got != "spot" {
		t.Errorf("attr info_type = %q", got)
	}

	var body map[string]any
	if err := json.Unmarshal(stub.lastData, &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["key_cd"] != "k-1" || body["country_cd"] != "JP" {
		t.Errorf("body = %v", body)
	}
	geom, ok := body["geometry"].(map[string]any)
	if !ok || geom["lat"] != 35.6 || geom["lng"] != 139.7 {
		t.Errorf("geometry = %v", body["geometry"])
	}
	if body["leave_date"] != "2026-04-23T09:00:00Z" {
		t.Errorf("leave_date = %v", body["leave_date"])
	}
}

func TestPublisher_PublishNewArrival_PropagatesError(t *testing.T) {
	t.Parallel()
	stub := &stubTopic{err: errors.New("pubsub 503")}
	pub := eventbus.New(stub)

	err := pub.PublishNewArrival(context.Background(), sampleEvent())
	if err == nil {
		t.Fatal("expected error to propagate")
	}
	// The wrapped error should still satisfy errors.Is against the original
	// transport error — assert that, instead of a vacuous kind check.
	if !errors.Is(err, stub.err) {
		t.Errorf("error chain does not include the underlying transport error: %v", err)
	}
}
