package observability

import (
	"context"
	"fmt"
	"runtime/debug"

	"connectrpc.com/connect"

	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

// RecoverInterceptor turns panics in Connect handlers into a KindInternal
// error so upstream clients receive a CodeInternal instead of a dropped
// connection. The panic is also logged with the stack trace.
func RecoverInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (resp connect.AnyResponse, err error) {
			defer func() {
				if r := recover(); r != nil {
					stack := debug.Stack()
					Logger(ctx).Error("panic recovered",
						"error.kind", string(errs.KindInternal),
						"error.message", fmt.Sprintf("%v", r),
						"error.stack", string(stack),
					)
					err = errs.Wrap("rpc.panic", errs.KindInternal, fmt.Errorf("%v", r))
				}
			}()
			return next(ctx, req)
		}
	}
}

// WrapJobRun wraps a job function with panic recovery and consistent logging.
// Intended for use by cmd/ingestion, cmd/notifier, cmd/setup.
func WrapJobRun(ctx context.Context, name string, run func(context.Context) error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			Logger(ctx).Error("job panic recovered",
				"job", name,
				"error.kind", string(errs.KindInternal),
				"error.message", fmt.Sprintf("%v", r),
				"error.stack", string(stack),
			)
			err = errs.Wrap("job.panic", errs.KindInternal, fmt.Errorf("%v", r))
		}
	}()
	return run(ctx)
}
