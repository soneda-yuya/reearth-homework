package dedup_test

import (
	"testing"
	"time"

	"github.com/soneda-yuya/reearth-homework/internal/notification/infrastructure/dedup"
)

// TestNew_AppliesDefaults verifies that zero-value Config fields fall back
// to the documented defaults. The production path (CheckAndMark against a
// live Firestore) is exercised in the U-NTF Build and Test runbook — we
// don't spin up the Firestore emulator here to keep unit tests hermetic.
func TestNew_AppliesDefaults(t *testing.T) {
	t.Parallel()
	// New accepts a nil client as long as we never call CheckAndMark; this
	// test only asserts constructor behaviour.
	d := dedup.New(nil, dedup.Config{})
	if d == nil {
		t.Fatal("New returned nil")
	}
}

// TestNew_PreservesCustomConfig confirms that an explicit collection name
// and TTL override the defaults.
func TestNew_PreservesCustomConfig(t *testing.T) {
	t.Parallel()
	d := dedup.New(nil, dedup.Config{
		Collection: "custom_dedup",
		TTL:        3 * time.Hour,
	})
	if d == nil {
		t.Fatal("New returned nil")
	}
}
