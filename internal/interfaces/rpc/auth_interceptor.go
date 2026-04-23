// Package rpc implements the Connect handlers and interceptors that sit
// between Flutter clients and the BFF use cases. Every Unary RPC flows
// through AuthInterceptor (enforces Firebase ID token) and ErrorInterceptor
// (maps errs.Kind → connect.Code) before it reaches a handler.
package rpc

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"connectrpc.com/connect"

	"github.com/soneda-yuya/overseas-safety-map/internal/shared/authctx"
	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
	"github.com/soneda-yuya/overseas-safety-map/internal/user/domain"
)

// NewAuthInterceptor returns a Unary interceptor that verifies the Firebase
// ID token in `Authorization: Bearer <idToken>` and injects the uid onto
// ctx via authctx.WithUID. A missing or invalid token aborts the call with
// errs.KindUnauthorized — ErrorInterceptor then converts that to
// connect.CodeUnauthenticated.
func NewAuthInterceptor(verifier domain.AuthVerifier, logger *slog.Logger) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			authHeader := req.Header().Get("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") {
				return nil, errs.Wrap("rpc.auth.missing_token", errs.KindUnauthorized,
					errors.New("missing bearer token"))
			}
			idToken := strings.TrimPrefix(authHeader, "Bearer ")
			uid, err := verifier.Verify(ctx, idToken)
			if err != nil {
				if logger != nil {
					logger.DebugContext(ctx, "auth rejected", "error", err.Error())
				}
				return nil, err
			}
			return next(authctx.WithUID(ctx, uid), req)
		}
	}
}
