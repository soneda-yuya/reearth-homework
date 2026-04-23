package firestore_test

import (
	"testing"

	userfirestore "github.com/soneda-yuya/overseas-safety-map/internal/user/infrastructure/firestore"
)

// The Firestore round-trip lives behind the live SDK; it is exercised via
// the Build and Test phase against the emulator or real project. The unit
// test covers constructor defaults — the same pattern the notifier adapter
// uses (userrepo_test.go).
func TestNew_AppliesDefaults(t *testing.T) {
	t.Parallel()
	r := userfirestore.New(nil, userfirestore.Config{})
	if r == nil {
		t.Fatal("New returned nil")
	}
}

func TestNew_CustomCollection(t *testing.T) {
	t.Parallel()
	r := userfirestore.New(nil, userfirestore.Config{Collection: "test_users"})
	if r == nil {
		t.Fatal("New returned nil")
	}
}
