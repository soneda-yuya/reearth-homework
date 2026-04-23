package fcm_test

import (
	"testing"

	"github.com/soneda-yuya/overseas-safety-map/internal/notification/infrastructure/fcm"
)

// TestNew_Smoke exercises the constructor. The real classification logic
// (Invalid vs Transient on per-token errors) is verified end-to-end via the
// Firebase Admin SDK in Build and Test — without a live Firebase project
// we can't mint the concrete FCM error types the SDK uses for IsRegistration
// TokenNotRegistered / IsInvalidArgument checks.
func TestNew_Smoke(t *testing.T) {
	t.Parallel()
	c := fcm.New(nil)
	if c == nil {
		t.Fatal("New returned nil")
	}
}
