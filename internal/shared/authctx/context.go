// Package authctx carries the authenticated user's uid on a context.Context.
// The auth interceptor populates it after verifying the Firebase ID token;
// downstream use cases read it with UIDFrom.
//
// The unexported key type prevents collisions with other packages using
// context.Context values.
package authctx

import "context"

type ctxKey struct{}

// WithUID returns a child context carrying uid. A nil ctx is normalised to
// context.Background() first so callers never receive a nil return, matching
// the defensive nil-guard in UIDFrom. If uid is empty the (normalised) ctx
// is returned unchanged — an empty uid never stands for "authenticated".
func WithUID(ctx context.Context, uid string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if uid == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, uid)
}

// UIDFrom returns the uid stored on ctx and whether it was present. The
// second return avoids forcing callers to check for the empty string.
func UIDFrom(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	uid, ok := ctx.Value(ctxKey{}).(string)
	return uid, ok && uid != ""
}
