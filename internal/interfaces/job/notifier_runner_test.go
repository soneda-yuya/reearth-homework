package job_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/soneda-yuya/overseas-safety-map/internal/interfaces/job"
	"github.com/soneda-yuya/overseas-safety-map/internal/notification/application"
	"github.com/soneda-yuya/overseas-safety-map/internal/notification/domain"
	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
)

// stubDecoder returns a canned event or a canned error. Simpler than the
// real pubsub envelope decoder and lets us test handler wiring in isolation.
type stubDecoder struct {
	event domain.NewArrivalEvent
	err   error
}

func (s stubDecoder) Decode(_ []byte) (domain.NewArrivalEvent, error) {
	return s.event, s.err
}

// stubDedup / stubUsers / stubFCM mirror the shapes fake_test.go uses, but
// duplicated here to keep the interfaces package independent of the
// application test internals.
type stubDedup struct {
	alreadySeen bool
	err         error
}

func (s stubDedup) CheckAndMark(_ context.Context, _ string) (bool, error) {
	return s.alreadySeen, s.err
}

type stubUsers struct {
	subs      []domain.UserProfile
	findErr   error
	removedOK bool
}

func (s *stubUsers) FindSubscribers(_ context.Context, _, _ string) ([]domain.UserProfile, error) {
	return s.subs, s.findErr
}
func (s *stubUsers) RemoveInvalidTokens(_ context.Context, _ string, _ []string) error {
	s.removedOK = true
	return nil
}

type stubFCM struct{}

func (stubFCM) SendMulticast(_ context.Context, msg domain.FCMMessage) (domain.BatchResult, error) {
	return domain.BatchResult{SuccessCount: len(msg.Tokens)}, nil
}

// newHandler wires a handler with stubs and an in-memory usecase.
func newHandler(decoder domain.EventDecoder, dedup domain.Dedup, users domain.UserRepository) *job.NotifierHandler {
	uc := application.NewDeliverNotificationUseCase(dedup, users, stubFCM{}, application.Deps{})
	return job.NewNotifierHandler(decoder, uc, nil)
}

func TestPush_Delivered_Returns200(t *testing.T) {
	t.Parallel()
	users := &stubUsers{subs: []domain.UserProfile{
		{
			UID: "u-1", FCMTokens: []string{"tok"},
			NotificationPreference: domain.NotificationPreference{Enabled: true},
		},
	}}
	h := newHandler(
		stubDecoder{event: domain.NewArrivalEvent{KeyCd: "k-1", CountryCd: "JP"}},
		stubDedup{},
		users,
	)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/pubsub/push", bytes.NewReader([]byte(`{}`)))
	h.Push(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestPush_DedupHit_Returns200(t *testing.T) {
	t.Parallel()
	h := newHandler(
		stubDecoder{event: domain.NewArrivalEvent{KeyCd: "k-1"}},
		stubDedup{alreadySeen: true},
		&stubUsers{},
	)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/pubsub/push", bytes.NewReader([]byte(`{}`)))
	h.Push(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (dedup hit)", rr.Code)
	}
}

func TestPush_NoSubscribers_Returns200(t *testing.T) {
	t.Parallel()
	h := newHandler(
		stubDecoder{event: domain.NewArrivalEvent{KeyCd: "k-1"}},
		stubDedup{},
		&stubUsers{subs: nil},
	)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/pubsub/push", bytes.NewReader([]byte(`{}`)))
	h.Push(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (no subscribers)", rr.Code)
	}
}

func TestPush_MalformedPayload_Returns400(t *testing.T) {
	t.Parallel()
	h := newHandler(
		stubDecoder{err: errs.Wrap("stub", errs.KindInvalidInput, errors.New("bad json"))},
		stubDedup{},
		&stubUsers{},
	)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/pubsub/push", bytes.NewReader([]byte(`not-json`)))
	h.Push(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestPush_TransientDedupError_Returns500(t *testing.T) {
	t.Parallel()
	h := newHandler(
		stubDecoder{event: domain.NewArrivalEvent{KeyCd: "k-1"}},
		stubDedup{err: errs.Wrap("stub", errs.KindExternal, errors.New("firestore 503"))},
		&stubUsers{},
	)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/pubsub/push", bytes.NewReader([]byte(`{}`)))
	h.Push(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

func TestPush_MethodNotAllowed(t *testing.T) {
	t.Parallel()
	h := newHandler(stubDecoder{}, stubDedup{}, &stubUsers{})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/pubsub/push", nil)
	h.Push(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rr.Code)
	}
}

func TestHealth(t *testing.T) {
	t.Parallel()
	h := newHandler(stubDecoder{}, stubDedup{}, &stubUsers{})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	h.Health(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if body := rr.Body.String(); body != "ok" {
		t.Errorf("body = %q, want ok", body)
	}
}
