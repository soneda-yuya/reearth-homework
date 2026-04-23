package observability_test

import (
	"context"
	"errors"
	"testing"

	"github.com/soneda-yuya/overseas-safety-map/internal/platform/observability"
	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
)

func setupForTest(t *testing.T) {
	t.Helper()
	shutdown, err := observability.Setup(context.Background(), observability.Config{
		ServiceName:  "test",
		Env:          "test",
		LogLevel:     "INFO",
		ExporterKind: "stdout",
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	t.Cleanup(func() { _ = shutdown(context.Background()) })
}

func TestLogger_ReturnsNonNil(t *testing.T) {
	setupForTest(t)
	if l := observability.Logger(context.Background()); l == nil {
		t.Fatal("Logger returned nil")
	}
}

func TestWith_AttachesAttribute(t *testing.T) {
	setupForTest(t)
	ctx := observability.With(context.Background(), "request_id", "abc123")
	if observability.Logger(ctx) == nil {
		t.Fatal("Logger returned nil after With")
	}
	// Adding another attribute on top should still produce a non-nil logger.
	ctx2 := observability.With(ctx, "key_cd", "69574")
	if observability.Logger(ctx2) == nil {
		t.Fatal("Logger returned nil after nested With")
	}
}

func TestWrapJobRun_RecoversFromPanic(t *testing.T) {
	setupForTest(t)
	err := observability.WrapJobRun(context.Background(), "test-job", func(ctx context.Context) error {
		panic("boom")
	})
	if err == nil {
		t.Fatal("expected error after panic")
	}
	if !errs.IsKind(err, errs.KindInternal) {
		t.Fatalf("want KindInternal, got %q (err=%v)", errs.KindOf(err), err)
	}
}

func TestWrapJobRun_PropagatesError(t *testing.T) {
	setupForTest(t)
	want := errors.New("normal error")
	err := observability.WrapJobRun(context.Background(), "test-job", func(ctx context.Context) error {
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("want %v, got %v", want, err)
	}
}
