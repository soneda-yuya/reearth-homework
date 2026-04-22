package retry_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/soneda-yuya/reearth-homework/internal/platform/retry"
	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

func TestDo_SuccessOnFirstAttempt(t *testing.T) {
	attempts := 0
	err := retry.Do(context.Background(), retry.Policy{MaxAttempts: 3, Initial: time.Nanosecond, Max: time.Millisecond, Multiplier: 2},
		func(ctx context.Context) error {
			attempts++
			return nil
		})
	if err != nil {
		t.Fatalf("Do returned err: %v", err)
	}
	if attempts != 1 {
		t.Errorf("want 1 attempt, got %d", attempts)
	}
}

func TestDo_RetriesExternalThenSucceeds(t *testing.T) {
	attempts := 0
	err := retry.Do(context.Background(), retry.Policy{MaxAttempts: 3, Initial: time.Nanosecond, Max: time.Millisecond, Multiplier: 2},
		func(ctx context.Context) error {
			attempts++
			if attempts < 2 {
				return errs.Wrap("op", errs.KindExternal, errors.New("503"))
			}
			return nil
		})
	if err != nil {
		t.Fatalf("Do returned err: %v", err)
	}
	if attempts != 2 {
		t.Errorf("want 2 attempts, got %d", attempts)
	}
}

func TestDo_StopsOnNonRetryable(t *testing.T) {
	attempts := 0
	err := retry.Do(context.Background(), retry.Policy{MaxAttempts: 5, Initial: time.Nanosecond, Max: time.Millisecond, Multiplier: 2},
		func(ctx context.Context) error {
			attempts++
			return errs.Wrap("op", errs.KindInvalidInput, errors.New("nope"))
		})
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts != 1 {
		t.Errorf("want 1 attempt (non-retryable), got %d", attempts)
	}
}

func TestDo_ExhaustsMaxAttempts(t *testing.T) {
	attempts := 0
	err := retry.Do(context.Background(), retry.Policy{MaxAttempts: 3, Initial: time.Nanosecond, Max: time.Millisecond, Multiplier: 2},
		func(ctx context.Context) error {
			attempts++
			return errs.Wrap("op", errs.KindExternal, errors.New("boom"))
		})
	if err == nil {
		t.Fatal("expected error after exhausting attempts")
	}
	if attempts != 3 {
		t.Errorf("want 3 attempts, got %d", attempts)
	}
}

func TestDo_HonoursContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := retry.Do(ctx, retry.DefaultPolicy, func(ctx context.Context) error {
		return errs.Wrap("op", errs.KindExternal, errors.New("boom"))
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestShouldRetry(t *testing.T) {
	if retry.ShouldRetry(nil) {
		t.Error("nil should not be retryable")
	}
	if retry.ShouldRetry(errors.New("plain")) {
		t.Error("plain error (unknown kind) should not be retryable by default")
	}
	if !retry.ShouldRetry(errs.Wrap("op", errs.KindExternal, errors.New("x"))) {
		t.Error("KindExternal should be retryable")
	}
	if retry.ShouldRetry(context.Canceled) {
		t.Error("context.Canceled should never be retried")
	}
}
