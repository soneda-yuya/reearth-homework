package firebasex_test

import (
	"context"
	"testing"

	"github.com/soneda-yuya/overseas-safety-map/internal/platform/firebasex"
	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
)

// TestNewApp_RequiresProjectID is the one guard we can check without a live
// Firebase project. Everything past the nil-check (firebase.NewApp) calls
// the Google IAM / metadata server and is covered by Build and Test.
func TestNewApp_RequiresProjectID(t *testing.T) {
	t.Parallel()
	_, err := firebasex.NewApp(context.Background(), firebasex.Config{})
	if err == nil {
		t.Fatal("expected error for empty ProjectID")
	}
	if !errs.IsKind(err, errs.KindInvalidInput) {
		t.Errorf("kind = %s, want KindInvalidInput", errs.KindOf(err))
	}
}
