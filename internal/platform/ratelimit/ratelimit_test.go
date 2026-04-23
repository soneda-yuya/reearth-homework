package ratelimit_test

import (
	"testing"

	"github.com/soneda-yuya/overseas-safety-map/internal/platform/ratelimit"
	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
)

func TestLimiter_AllowsInitialBurst(t *testing.T) {
	l := ratelimit.New(60, 3, "test")
	for i := 0; i < 3; i++ {
		if err := l.Allow(); err != nil {
			t.Errorf("burst request %d should succeed: %v", i, err)
		}
	}
	if err := l.Allow(); err == nil {
		t.Errorf("request beyond burst should fail")
	} else if !errs.IsKind(err, errs.KindExternal) {
		t.Errorf("want KindExternal, got %q", errs.KindOf(err))
	}
}

func TestLimiter_DefaultSafeForInvalidArgs(t *testing.T) {
	// Should not panic and should accept at least one request.
	l := ratelimit.New(0, 0, "test")
	if err := l.Allow(); err != nil {
		t.Errorf("default limiter should allow first request: %v", err)
	}
}

func TestLimiter_Name(t *testing.T) {
	l := ratelimit.New(10, 1, "claude")
	if got := l.Name(); got != "claude" {
		t.Errorf("Name = %q, want claude", got)
	}
}
