package application_test

import (
	"context"
	"testing"
	"time"

	"github.com/soneda-yuya/reearth-homework/internal/notification/application"
	"github.com/soneda-yuya/reearth-homework/internal/notification/domain"
	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

func sampleEvent() domain.NewArrivalEvent {
	return domain.NewArrivalEvent{
		KeyCd:     "k-1",
		CountryCd: "JP",
		InfoType:  "spot",
		Title:     "注意喚起",
		Lead:      "パリ市内でデモ",
		LeaveDate: time.Date(2026, 4, 23, 10, 0, 0, 0, time.UTC),
	}
}

func subscriber(uid string, tokens ...string) domain.UserProfile {
	return domain.UserProfile{
		UID:       uid,
		FCMTokens: tokens,
		NotificationPreference: domain.NotificationPreference{
			Enabled:          true,
			TargetCountryCds: []string{"JP"},
		},
	}
}

// Scenario 1: happy path — delivered to subscribers, all tokens succeed.
func TestExecute_Delivered(t *testing.T) {
	t.Parallel()
	dedup := newFakeDedup()
	users := newFakeUserRepo()
	users.subscribers["JP"] = []domain.UserProfile{
		subscriber("u-1", "tok-a", "tok-b"),
		subscriber("u-2", "tok-c"),
	}
	fcm := &fakeFCM{}

	uc := application.NewDeliverNotificationUseCase(dedup, users, fcm, application.Deps{Concurrency: 2})
	res, err := uc.Execute(context.Background(), application.DeliverInput{Event: sampleEvent()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Outcome != application.OutcomeDelivered {
		t.Errorf("Outcome = %s, want delivered", res.Outcome)
	}
	if res.RecipientsCount != 2 || res.FCMSuccessTokens != 3 {
		t.Errorf("result = %+v, want Recipients=2 Success=3", res)
	}
	if len(fcm.calls) != 2 {
		t.Errorf("fcm calls = %d, want 2", len(fcm.calls))
	}
}

// Scenario 2: Dedup hit — early 200 return, no FCM, no Firestore read.
func TestExecute_Deduped(t *testing.T) {
	t.Parallel()
	dedup := newFakeDedup()
	dedup.seen["k-1"] = true // pre-mark
	users := newFakeUserRepo()
	fcm := &fakeFCM{}

	uc := application.NewDeliverNotificationUseCase(dedup, users, fcm, application.Deps{})
	res, err := uc.Execute(context.Background(), application.DeliverInput{Event: sampleEvent()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Outcome != application.OutcomeDeduped {
		t.Errorf("Outcome = %s, want deduped", res.Outcome)
	}
	if len(fcm.calls) != 0 {
		t.Errorf("fcm should not be called, got %d", len(fcm.calls))
	}
}

// Scenario 3: no subscribers — 200 return, no FCM.
func TestExecute_NoSubscribers(t *testing.T) {
	t.Parallel()
	dedup := newFakeDedup()
	users := newFakeUserRepo() // empty
	fcm := &fakeFCM{}

	uc := application.NewDeliverNotificationUseCase(dedup, users, fcm, application.Deps{})
	res, err := uc.Execute(context.Background(), application.DeliverInput{Event: sampleEvent()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Outcome != application.OutcomeNoSubscribers {
		t.Errorf("Outcome = %s, want no_subscribers", res.Outcome)
	}
	if len(fcm.calls) != 0 {
		t.Errorf("fcm should not be called")
	}
}

// Scenario 4: partial FCM failure — some tokens invalid, removed from Firestore.
// Overall still counts as delivered (200 to Pub/Sub).
func TestExecute_PartialFailureRemovesInvalidTokens(t *testing.T) {
	t.Parallel()
	dedup := newFakeDedup()
	users := newFakeUserRepo()
	users.subscribers["JP"] = []domain.UserProfile{
		subscriber("u-1", "tok-valid", "tok-dead"),
	}
	fcm := &fakeFCM{}
	fcm.resultFn = func(msg domain.FCMMessage) (domain.BatchResult, error) {
		return domain.BatchResult{
			SuccessCount: 1,
			FailureCount: 1,
			Invalid:      []string{"tok-dead"},
		}, nil
	}

	uc := application.NewDeliverNotificationUseCase(dedup, users, fcm, application.Deps{})
	res, err := uc.Execute(context.Background(), application.DeliverInput{Event: sampleEvent()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Outcome != application.OutcomeDelivered {
		t.Errorf("Outcome = %s, want delivered", res.Outcome)
	}
	if res.FCMSuccessTokens != 1 || res.FCMFailedTokens != 1 || res.InvalidTokensRemoved != 1 {
		t.Errorf("result = %+v", res)
	}
	if removed := users.removed["u-1"]; len(removed) != 1 || removed[0] != "tok-dead" {
		t.Errorf("removed = %v, want [tok-dead]", removed)
	}
}

// Scenario 5: transient error during dedup — Execute returns error so the
// handler can respond 500 and Pub/Sub retries.
func TestExecute_DedupTransientErrorPropagates(t *testing.T) {
	t.Parallel()
	dedup := newFakeDedup()
	dedup.err = transient("firestore 503")
	users := newFakeUserRepo()
	fcm := &fakeFCM{}

	uc := application.NewDeliverNotificationUseCase(dedup, users, fcm, application.Deps{})
	_, err := uc.Execute(context.Background(), application.DeliverInput{Event: sampleEvent()})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errs.IsKind(err, errs.KindExternal) {
		t.Errorf("kind = %s, want KindExternal", errs.KindOf(err))
	}
}

// Bonus: users.Find transient error propagates as 500.
func TestExecute_ResolveTransientErrorPropagates(t *testing.T) {
	t.Parallel()
	dedup := newFakeDedup()
	users := newFakeUserRepo()
	users.findErr = transient("firestore timeout")
	fcm := &fakeFCM{}

	uc := application.NewDeliverNotificationUseCase(dedup, users, fcm, application.Deps{})
	_, err := uc.Execute(context.Background(), application.DeliverInput{Event: sampleEvent()})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errs.IsKind(err, errs.KindExternal) {
		t.Errorf("kind = %s, want KindExternal", errs.KindOf(err))
	}
}

// DeliverOutcome.String は wire name で ログ / metric attribute に使うので
// 契約として検証。
func TestDeliverOutcome_String(t *testing.T) {
	t.Parallel()
	tests := []struct {
		o    application.DeliverOutcome
		want string
	}{
		{application.OutcomeDelivered, "delivered"},
		{application.OutcomeDeduped, "deduped"},
		{application.OutcomeNoSubscribers, "no_subscribers"},
	}
	for _, tc := range tests {
		if got := tc.o.String(); got != tc.want {
			t.Errorf("String() = %q, want %q", got, tc.want)
		}
	}
}
