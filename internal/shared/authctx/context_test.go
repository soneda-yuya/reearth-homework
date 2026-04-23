package authctx_test

import (
	"context"
	"testing"

	"github.com/soneda-yuya/overseas-safety-map/internal/shared/authctx"
)

func TestWithUIDAndUIDFrom(t *testing.T) {
	t.Parallel()
	ctx := authctx.WithUID(context.Background(), "uid-1")
	got, ok := authctx.UIDFrom(ctx)
	if !ok || got != "uid-1" {
		t.Errorf("UIDFrom after WithUID = (%q, %v); want (uid-1, true)", got, ok)
	}
}

func TestWithUIDEmptyIsNoop(t *testing.T) {
	t.Parallel()
	parent := context.Background()
	ctx := authctx.WithUID(parent, "")
	if _, ok := authctx.UIDFrom(ctx); ok {
		t.Error("empty uid must not populate ctx — an anonymous request is unauthenticated")
	}
}

func TestUIDFromNilContext(t *testing.T) {
	t.Parallel()
	// Deliberately pass a typed-nil Context to exercise the defensive
	// nil-guard in UIDFrom. staticcheck warns against literal nil, but the
	// guard exists precisely for this shape.
	var ctx context.Context
	if _, ok := authctx.UIDFrom(ctx); ok { //nolint:staticcheck // SA1012: exercising the nil guard
		t.Error("nil ctx should yield ok=false")
	}
}

func TestUIDFromAbsent(t *testing.T) {
	t.Parallel()
	if _, ok := authctx.UIDFrom(context.Background()); ok {
		t.Error("ctx without WithUID must report ok=false")
	}
}
