// Package retry provides exponential backoff with jitter for idempotent
// operations against external services.
//
// Use Do only for idempotent calls (GET, HEAD, LLM inference). POST/PATCH
// without an idempotency key must not be retried.
package retry

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"

	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

// Policy controls retry behaviour.
type Policy struct {
	MaxAttempts int
	Initial     time.Duration
	Max         time.Duration
	Multiplier  float64
	Jitter      float64 // 0.25 means ±25%
}

// DefaultPolicy matches NFR Design: 3 attempts, 500ms initial, 8s cap, ±25% jitter.
var DefaultPolicy = Policy{
	MaxAttempts: 3,
	Initial:     500 * time.Millisecond,
	Max:         8 * time.Second,
	Multiplier:  2.0,
	Jitter:      0.25,
}

// ShouldRetry returns true for errors we consider transient. Extend as needed
// by calling code (e.g. HTTP adapters can layer on 5xx / 429 checks and then
// defer to this function).
func ShouldRetry(err error) bool {
	if err == nil {
		return false
	}
	// Don't retry cancelled / deadline contexts; caller intent wins.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// External errors are transient; internal errors typically are not.
	return errs.IsKind(err, errs.KindExternal)
}

// Do runs op up to policy.MaxAttempts times, sleeping between attempts
// according to the exponential-backoff-with-jitter formula. The first attempt
// runs immediately.
//
// Do returns early if ctx is cancelled, or if op returns a non-retryable
// error (per ShouldRetry).
func Do(ctx context.Context, policy Policy, op func(context.Context) error) error {
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = 1
	}
	delay := policy.Initial
	var lastErr error

	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := op(ctx)
		if err == nil {
			return nil
		}
		lastErr = err
		if !ShouldRetry(err) || attempt == policy.MaxAttempts {
			return err
		}
		sleep := jitter(delay, policy.Jitter)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(sleep):
		}
		delay = nextDelay(delay, policy)
	}
	return lastErr
}

func nextDelay(cur time.Duration, p Policy) time.Duration {
	next := time.Duration(float64(cur) * p.Multiplier)
	if next > p.Max {
		return p.Max
	}
	return next
}

// jitter adds a symmetric random offset of ±(ratio * d).
func jitter(d time.Duration, ratio float64) time.Duration {
	if ratio <= 0 {
		return d
	}
	delta := float64(d) * ratio
	offset := (rand.Float64()*2 - 1) * delta
	return d + time.Duration(offset)
}
