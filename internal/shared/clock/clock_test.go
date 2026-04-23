package clock_test

import (
	"testing"
	"time"

	"github.com/soneda-yuya/overseas-safety-map/internal/shared/clock"
)

func TestSystemClock_ReturnsUTC(t *testing.T) {
	c := clock.System()
	got := c.Now()
	if _, off := got.Zone(); off != 0 {
		t.Errorf("SystemClock.Now should return UTC, got offset %d seconds", off)
	}
}

func TestSystemClock_Monotonic(t *testing.T) {
	c := clock.System()
	a := c.Now()
	time.Sleep(1 * time.Millisecond)
	b := c.Now()
	if !b.After(a) {
		t.Errorf("SystemClock should advance: a=%v b=%v", a, b)
	}
}

func TestFixedClock_AlwaysSameTime(t *testing.T) {
	pinned := time.Date(2026, 4, 22, 9, 0, 0, 0, time.UTC)
	c := clock.Fixed(pinned)
	if got := c.Now(); !got.Equal(pinned) {
		t.Errorf("FixedClock.Now = %v, want %v", got, pinned)
	}
	if got := c.Now(); !got.Equal(pinned) {
		t.Errorf("FixedClock.Now should stay constant across calls")
	}
}

func TestFixedClock_NormalizesToUTC(t *testing.T) {
	jst := time.FixedZone("JST", 9*3600)
	input := time.Date(2026, 4, 22, 18, 0, 0, 0, jst)
	c := clock.Fixed(input)
	got := c.Now()
	if _, off := got.Zone(); off != 0 {
		t.Errorf("FixedClock.Now should normalise to UTC, got offset %d", off)
	}
	if !got.Equal(input) {
		t.Errorf("FixedClock should preserve the instant (got %v, want %v)", got, input)
	}
}
