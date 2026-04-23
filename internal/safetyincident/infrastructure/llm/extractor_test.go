package llm_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/soneda-yuya/reearth-homework/internal/safetyincident/domain"
	infralm "github.com/soneda-yuya/reearth-homework/internal/safetyincident/infrastructure/llm"
	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

type stubCompleter struct {
	resp string
	err  error
	last struct{ system, user string }
}

func (s *stubCompleter) Complete(_ context.Context, system, user string) (string, error) {
	s.last.system = system
	s.last.user = user
	return s.resp, s.err
}

func sampleItem() domain.MailItem {
	return domain.MailItem{
		KeyCd:       "k-1",
		Title:       "(注意喚起) ジャカルタ南部における強盗事件",
		Lead:        "ジャカルタ南部で日本人を狙った強盗事件が発生しました。",
		MainText:    "本日深夜、ジャカルタ南部の繁華街において...",
		CountryCd:   "ID",
		CountryName: "インドネシア",
		AreaName:    "アジア",
		LeaveDate:   time.Date(2026, 4, 23, 7, 15, 0, 0, time.UTC),
	}
}

func TestExtractor_HappyPath(t *testing.T) {
	t.Parallel()
	stub := &stubCompleter{resp: `{"location":"ジャカルタ南部","confidence":0.85}`}
	ex := infralm.New(stub)

	got, err := ex.Extract(context.Background(), sampleItem())
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if got.Location != "ジャカルタ南部" {
		t.Errorf("Location = %q", got.Location)
	}
	if got.Confidence != 0.85 {
		t.Errorf("Confidence = %v", got.Confidence)
	}
	// Spot-check the user prompt mentions key fields so prompt drift fails the test.
	if !contains(stub.last.user, "ジャカルタ南部") {
		t.Errorf("user prompt missing main_text; got %q", stub.last.user)
	}
	if !contains(stub.last.user, "ID") {
		t.Errorf("user prompt missing country_cd; got %q", stub.last.user)
	}
}

func TestExtractor_TolerantOfCodeFenceWrapper(t *testing.T) {
	t.Parallel()
	wrapped := "```json\n{\"location\":\"パリ 9 区\",\"confidence\":0.9}\n```"
	ex := infralm.New(&stubCompleter{resp: wrapped})

	got, err := ex.Extract(context.Background(), sampleItem())
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if got.Location != "パリ 9 区" {
		t.Errorf("Location = %q", got.Location)
	}
}

func TestExtractor_MalformedJSON_FallsThroughToEmpty(t *testing.T) {
	t.Parallel()
	ex := infralm.New(&stubCompleter{resp: "I'm sorry, I can't help with that."})

	got, err := ex.Extract(context.Background(), sampleItem())
	if err != nil {
		t.Fatalf("Extract: %v (should NOT be a hard error)", err)
	}
	if got.Location != "" || got.Confidence != 0 {
		t.Errorf("expected empty fallback, got %+v", got)
	}
}

func TestExtractor_TransportErrorPropagates(t *testing.T) {
	t.Parallel()
	transport := errs.Wrap("stub", errs.KindUnauthorized, errors.New("401"))
	ex := infralm.New(&stubCompleter{err: transport})

	_, err := ex.Extract(context.Background(), sampleItem())
	if err == nil {
		t.Fatal("expected error to propagate")
	}
	if !errs.IsKind(err, errs.KindUnauthorized) {
		t.Errorf("kind = %s, want KindUnauthorized", errs.KindOf(err))
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && (indexOf(haystack, needle) >= 0))
}

func indexOf(s, sub string) int {
outer:
	for i := 0; i+len(sub) <= len(s); i++ {
		for j := 0; j < len(sub); j++ {
			if s[i+j] != sub[j] {
				continue outer
			}
		}
		return i
	}
	return -1
}
