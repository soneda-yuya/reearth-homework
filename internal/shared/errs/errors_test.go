package errs_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

func TestWrap_NilReturnsNil(t *testing.T) {
	if got := errs.Wrap("op", errs.KindInternal, nil); got != nil {
		t.Fatalf("Wrap(nil) = %v, want nil", got)
	}
}

func TestWrap_MessageIncludesOpAndKind(t *testing.T) {
	err := errs.Wrap("cms.repository.get", errs.KindNotFound, errors.New("boom"))
	msg := err.Error()
	for _, sub := range []string{"cms.repository.get", "not_found", "boom"} {
		if !strings.Contains(msg, sub) {
			t.Errorf("error message %q missing %q", msg, sub)
		}
	}
}

func TestKindOf_UnknownWhenNil(t *testing.T) {
	if got := errs.KindOf(nil); got != errs.KindUnknown {
		t.Fatalf("KindOf(nil) = %q, want unknown", got)
	}
}

func TestKindOf_UnknownWhenNotAppError(t *testing.T) {
	if got := errs.KindOf(errors.New("plain")); got != errs.KindUnknown {
		t.Fatalf("KindOf(plain) = %q, want unknown", got)
	}
}

func TestIsKind_FindsAcrossChain(t *testing.T) {
	inner := errs.Wrap("inner", errs.KindExternal, errors.New("mapbox 500"))
	outer := errs.Wrap("outer", errs.KindInternal, inner)
	if !errs.IsKind(outer, errs.KindExternal) {
		t.Errorf("IsKind should see KindExternal through the chain")
	}
	if !errs.IsKind(outer, errs.KindInternal) {
		t.Errorf("IsKind should see the outer KindInternal")
	}
	if errs.IsKind(outer, errs.KindConflict) {
		t.Errorf("IsKind should not match kinds absent from the chain")
	}
}

func TestKindOf_ReturnsOutermost(t *testing.T) {
	inner := errs.Wrap("inner", errs.KindExternal, errors.New("boom"))
	outer := errs.Wrap("outer", errs.KindInternal, inner)
	if got := errs.KindOf(outer); got != errs.KindInternal {
		t.Errorf("KindOf outer = %q, want internal", got)
	}
}

func TestUnwrap_IsCompatibleWithErrorsIs(t *testing.T) {
	sentinel := errors.New("sentinel")
	wrapped := errs.Wrap("op", errs.KindInternal, sentinel)
	if !errors.Is(wrapped, sentinel) {
		t.Errorf("errors.Is should traverse AppError to the sentinel")
	}
}

// Property: for any non-nil error, KindOf(Wrap(op, k, err)) == k.
func TestProp_KindOfAfterWrapReturnsProvidedKind(t *testing.T) {
	kinds := []errs.Kind{
		errs.KindNotFound, errs.KindInvalidInput, errs.KindUnauthorized,
		errs.KindPermissionDenied, errs.KindExternal, errs.KindConflict,
		errs.KindInternal,
	}
	rapid.Check(t, func(t *rapid.T) {
		op := rapid.String().Draw(t, "op")
		kind := kinds[rapid.IntRange(0, len(kinds)-1).Draw(t, "kind-idx")]
		cause := rapid.String().Draw(t, "cause")
		err := errs.Wrap(op, kind, errors.New(cause))
		if got := errs.KindOf(err); got != kind {
			t.Fatalf("KindOf(Wrap(%q, %q, ...)) = %q", op, kind, got)
		}
		if !errs.IsKind(err, kind) {
			t.Fatalf("IsKind failed for kind %q", kind)
		}
	})
}

// Property: Wrap preserves the cause so it is reachable through errors.Unwrap.
func TestProp_WrapChainPreservesCause(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		depth := rapid.IntRange(1, 5).Draw(t, "depth")
		cause := fmt.Errorf("leaf-%d", rapid.Int().Draw(t, "seed"))
		cur := error(cause)
		for i := 0; i < depth; i++ {
			cur = errs.Wrap(fmt.Sprintf("op-%d", i), errs.KindInternal, cur)
		}
		if !errors.Is(cur, cause) {
			t.Fatalf("cause not reachable through %d Wrap layers", depth)
		}
	})
}
