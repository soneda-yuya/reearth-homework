package rpc_test

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"

	"github.com/soneda-yuya/overseas-safety-map/internal/interfaces/rpc"
	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
)

// makeNext returns a Unary handler that ignores its input and yields the
// supplied error (or nil). A Unary interceptor is exercised by calling
// inter(handler) and then invoking the returned function.
func makeNext(err error) connect.UnaryFunc {
	return func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) {
		return nil, err
	}
}

func runIntercept(t *testing.T, inter connect.UnaryInterceptorFunc, err error) error {
	t.Helper()
	handler := inter(makeNext(err))
	_, got := handler(context.Background(), &noopReq{})
	return got
}

// noopReq is the minimum AnyRequest implementation the interceptor needs:
// it only touches Header() on the auth path (unused here).
type noopReq struct {
	connect.Request[struct{}]
}

func TestErrorInterceptor_KindMapping(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		kind errs.Kind
		want connect.Code
	}{
		{"not found", errs.KindNotFound, connect.CodeNotFound},
		{"invalid input", errs.KindInvalidInput, connect.CodeInvalidArgument},
		{"unauthorized", errs.KindUnauthorized, connect.CodeUnauthenticated},
		{"permission denied", errs.KindPermissionDenied, connect.CodePermissionDenied},
		{"conflict", errs.KindConflict, connect.CodeAlreadyExists},
		{"external", errs.KindExternal, connect.CodeUnavailable},
		{"internal", errs.KindInternal, connect.CodeInternal},
		{"unknown falls back to internal", errs.KindUnknown, connect.CodeInternal},
	}
	inter := rpc.NewErrorInterceptor("dev")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := errs.Wrap("op", tc.kind, errors.New("boom"))
			got := runIntercept(t, inter, err)
			if connect.CodeOf(got) != tc.want {
				t.Errorf("Code = %v; want %v", connect.CodeOf(got), tc.want)
			}
		})
	}
}

func TestErrorInterceptor_ProdMasksInternal(t *testing.T) {
	t.Parallel()
	inter := rpc.NewErrorInterceptor("prod")
	err := errs.Wrap("op", errs.KindInternal, errors.New("db password=secret"))
	got := runIntercept(t, inter, err)
	var ce *connect.Error
	if !errors.As(got, &ce) {
		t.Fatalf("not a connect.Error: %T %v", got, got)
	}
	if ce.Message() == "" || ce.Message() == "db password=secret" {
		t.Errorf("prod must mask internal message; got %q", ce.Message())
	}
}

func TestErrorInterceptor_DevKeepsMessage(t *testing.T) {
	t.Parallel()
	inter := rpc.NewErrorInterceptor("dev")
	err := errs.Wrap("op", errs.KindInternal, errors.New("root cause"))
	got := runIntercept(t, inter, err)
	var ce *connect.Error
	if !errors.As(got, &ce) {
		t.Fatalf("not a connect.Error: %T %v", got, got)
	}
	if ce.Message() == "" || ce.Message() == "internal server error" {
		t.Errorf("dev must preserve message; got %q", ce.Message())
	}
}

func TestErrorInterceptor_NilPassThrough(t *testing.T) {
	t.Parallel()
	inter := rpc.NewErrorInterceptor("dev")
	if got := runIntercept(t, inter, nil); got != nil {
		t.Errorf("nil err must pass through; got %v", got)
	}
}

func TestErrorInterceptor_PreservesExistingConnectError(t *testing.T) {
	t.Parallel()
	inter := rpc.NewErrorInterceptor("prod")
	src := connect.NewError(connect.CodeResourceExhausted, errors.New("rate limited"))
	got := runIntercept(t, inter, src)
	if connect.CodeOf(got) != connect.CodeResourceExhausted {
		t.Errorf("pre-built connect.Error code lost: %v", connect.CodeOf(got))
	}
}
