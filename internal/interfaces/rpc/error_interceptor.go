package rpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
)

// ProdEnv is the PLATFORM_ENV value that triggers message masking. Anything
// else keeps raw error text for dev / staging debuggability.
const ProdEnv = "prod"

// NewErrorInterceptor maps errs.Kind → connect.Code on every RPC response.
// In prod mode, CodeInternal / CodeUnavailable messages are replaced with a
// generic text so callers can't fingerprint internals (the original error is
// still logged server-side via observability).
func NewErrorInterceptor(env string) connect.UnaryInterceptorFunc {
	maskInternal := env == ProdEnv
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			resp, err := next(ctx, req)
			if err == nil {
				return resp, nil
			}
			// Preserve explicit connect.Error — interceptors further down
			// the chain (or handlers themselves) may already have produced
			// one.
			var connectErr *connect.Error
			if errors.As(err, &connectErr) {
				return nil, connectErr
			}
			code := kindToCode(errs.KindOf(err))
			msg := err
			if maskInternal && (code == connect.CodeInternal || code == connect.CodeUnavailable) {
				msg = errors.New("internal server error")
			}
			return nil, connect.NewError(code, msg)
		}
	}
}

// kindToCode is the canonical error kind → Connect code map. Keep in sync
// with errs.Kind declarations.
func kindToCode(kind errs.Kind) connect.Code {
	switch kind {
	case errs.KindNotFound:
		return connect.CodeNotFound
	case errs.KindInvalidInput:
		return connect.CodeInvalidArgument
	case errs.KindUnauthorized:
		return connect.CodeUnauthenticated
	case errs.KindPermissionDenied:
		return connect.CodePermissionDenied
	case errs.KindConflict:
		return connect.CodeAlreadyExists
	case errs.KindExternal:
		return connect.CodeUnavailable
	case errs.KindInternal, errs.KindUnknown:
		return connect.CodeInternal
	default:
		return connect.CodeInternal
	}
}
