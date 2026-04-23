// Package ratelimit wraps golang.org/x/time/rate with an error-returning
// Allow helper that slots into adapter code cleanly.
package ratelimit

import (
	"context"
	"fmt"

	"golang.org/x/time/rate"

	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

// Limiter allows at most rpm requests per minute with a burst. Concurrency-safe.
type Limiter struct {
	inner *rate.Limiter
	name  string
}

// New returns a Limiter configured from a requests-per-minute value.
// name identifies the limiter in error messages.
func New(rpm, burst int, name string) *Limiter {
	if rpm <= 0 {
		rpm = 1
	}
	if burst <= 0 {
		burst = 1
	}
	// rate.Limit is tokens per second; divide by 60 to convert from RPM.
	return &Limiter{
		inner: rate.NewLimiter(rate.Limit(float64(rpm)/60.0), burst),
		name:  name,
	}
}

// Allow consumes a token without blocking. When the bucket is empty it
// returns an external-kind error so upstream retry logic can decide whether
// to back off or surface a failure immediately.
func (l *Limiter) Allow() error {
	if l.inner.Allow() {
		return nil
	}
	return errs.Wrap("ratelimit", errs.KindExternal,
		fmt.Errorf("%s: rate limit exceeded", l.name))
}

// Wait blocks until one token is available or ctx is cancelled. Use this in
// adapters that need to throttle outgoing calls without dropping the request
// (the bucket fills back up over time).
func (l *Limiter) Wait(ctx context.Context) error {
	if err := l.inner.Wait(ctx); err != nil {
		return errs.Wrap("ratelimit", errs.KindExternal,
			fmt.Errorf("%s: %w", l.name, err))
	}
	return nil
}

// Name returns the limiter's identifier (used in logs).
func (l *Limiter) Name() string { return l.name }
