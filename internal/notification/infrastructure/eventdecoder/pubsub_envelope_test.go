package eventdecoder_test

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/soneda-yuya/reearth-homework/internal/notification/infrastructure/eventdecoder"
	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

func buildEnvelope(t *testing.T, inner map[string]any, attrs map[string]string) []byte {
	t.Helper()
	innerJSON, err := json.Marshal(inner)
	if err != nil {
		t.Fatalf("marshal inner: %v", err)
	}
	env := map[string]any{
		"message": map[string]any{
			"data":       base64.StdEncoding.EncodeToString(innerJSON),
			"attributes": attrs,
			"messageId":  "msg-1",
		},
		"subscription": "projects/p/subscriptions/s",
	}
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return b
}

func TestDecode_HappyPath(t *testing.T) {
	t.Parallel()
	body := buildEnvelope(t,
		map[string]any{
			"key_cd":     "k-1",
			"country_cd": "JP",
			"info_type":  "spot",
			"title":      "title",
			"lead":       "lead",
			"leave_date": "2026-04-23T09:00:00Z",
		},
		map[string]string{"key_cd": "k-1"},
	)

	ev, err := eventdecoder.New().Decode(body)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if ev.KeyCd != "k-1" || ev.CountryCd != "JP" || ev.InfoType != "spot" {
		t.Errorf("event = %+v", ev)
	}
	if ev.LeaveDate.IsZero() {
		t.Error("LeaveDate should be set")
	}
}

func TestDecode_FallsBackToAttributes(t *testing.T) {
	t.Parallel()
	body := buildEnvelope(t,
		map[string]any{"title": "no key_cd in body"}, // body omits key_cd
		map[string]string{"key_cd": "from-attrs", "country_cd": "FR", "info_type": "danger"},
	)
	ev, err := eventdecoder.New().Decode(body)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if ev.KeyCd != "from-attrs" || ev.CountryCd != "FR" || ev.InfoType != "danger" {
		t.Errorf("attributes not applied: %+v", ev)
	}
}

func TestDecode_MalformedEnvelope(t *testing.T) {
	t.Parallel()
	_, err := eventdecoder.New().Decode([]byte("not-json"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !errs.IsKind(err, errs.KindInvalidInput) {
		t.Errorf("kind = %s, want KindInvalidInput", errs.KindOf(err))
	}
}

func TestDecode_EmptyData(t *testing.T) {
	t.Parallel()
	env := `{"message":{"data":"","attributes":null}}`
	_, err := eventdecoder.New().Decode([]byte(env))
	if err == nil {
		t.Fatal("expected error")
	}
	if !errs.IsKind(err, errs.KindInvalidInput) {
		t.Errorf("kind = %s, want KindInvalidInput", errs.KindOf(err))
	}
}

func TestDecode_BadBase64(t *testing.T) {
	t.Parallel()
	env := `{"message":{"data":"!!!not-base64!!!","attributes":null}}`
	_, err := eventdecoder.New().Decode([]byte(env))
	if err == nil {
		t.Fatal("expected error")
	}
	if !errs.IsKind(err, errs.KindInvalidInput) {
		t.Errorf("kind = %s, want KindInvalidInput", errs.KindOf(err))
	}
}

func TestDecode_BadInnerJSON(t *testing.T) {
	t.Parallel()
	env := map[string]any{
		"message": map[string]any{
			"data": base64.StdEncoding.EncodeToString([]byte("not-json")),
		},
	}
	b, _ := json.Marshal(env)
	_, err := eventdecoder.New().Decode(b)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errs.IsKind(err, errs.KindInvalidInput) {
		t.Errorf("kind = %s, want KindInvalidInput", errs.KindOf(err))
	}
}

func TestDecode_MissingKeyCd(t *testing.T) {
	t.Parallel()
	body := buildEnvelope(t,
		map[string]any{"country_cd": "JP"}, // no key_cd anywhere
		nil,
	)
	_, err := eventdecoder.New().Decode(body)
	if err == nil {
		t.Fatal("expected error for missing key_cd")
	}
	if !errs.IsKind(err, errs.KindInvalidInput) {
		t.Errorf("kind = %s, want KindInvalidInput", errs.KindOf(err))
	}
}

func TestDecode_BadLeaveDate(t *testing.T) {
	t.Parallel()
	body := buildEnvelope(t,
		map[string]any{"key_cd": "k-1", "leave_date": "not-rfc3339"},
		nil,
	)
	_, err := eventdecoder.New().Decode(body)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errs.IsKind(err, errs.KindInvalidInput) {
		t.Errorf("kind = %s, want KindInvalidInput", errs.KindOf(err))
	}
}
