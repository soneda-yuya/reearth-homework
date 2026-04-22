// Package clock abstracts time.Now so code that needs the current time can be
// tested deterministically.
//
// Production code should accept a Clock via dependency injection instead of
// calling time.Now() directly. Tests inject a FixedClock to make time
// predictable.
package clock

import "time"

// Clock returns the current time.
type Clock interface {
	Now() time.Time
}

// SystemClock uses time.Now() and normalises to UTC.
type SystemClock struct{}

// Now returns the current wall-clock time in UTC.
func (SystemClock) Now() time.Time { return time.Now().UTC() }

// System returns a ready-to-use SystemClock. Use this in cmd/*/main.go so
// dependencies can be passed a Clock interface rather than a concrete type.
func System() Clock { return SystemClock{} }

// FixedClock returns the same time on every call. Useful in tests.
type FixedClock struct {
	FixedTime time.Time
}

// Now returns the fixed time in UTC.
func (c FixedClock) Now() time.Time { return c.FixedTime.UTC() }

// Fixed returns a FixedClock pinned to the given time.
func Fixed(t time.Time) Clock { return FixedClock{FixedTime: t} }
