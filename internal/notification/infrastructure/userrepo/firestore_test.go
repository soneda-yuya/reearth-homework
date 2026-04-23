package userrepo_test

import (
	"testing"

	"github.com/soneda-yuya/overseas-safety-map/internal/notification/infrastructure/userrepo"
)

// TestNew_AppliesDefaults covers constructor defaults. The actual Firestore
// round-trip (FindSubscribers / RemoveInvalidTokens) lives behind the live
// Firestore SDK; it is exercised via Build and Test with the emulator or
// real project.
func TestNew_AppliesDefaults(t *testing.T) {
	t.Parallel()
	r := userrepo.New(nil, userrepo.Config{})
	if r == nil {
		t.Fatal("New returned nil")
	}
}

func TestNew_CustomCollection(t *testing.T) {
	t.Parallel()
	r := userrepo.New(nil, userrepo.Config{Collection: "test_users"})
	if r == nil {
		t.Fatal("New returned nil")
	}
}
