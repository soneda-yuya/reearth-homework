// Package errs defines the application error type and the kinds used across the codebase.
//
// Wrap external errors with [Wrap] at the boundary where the meaning becomes clear
// (typically at the infrastructure adapter or the application service).
//
//	if err := cms.Get(ctx, id); err != nil {
//	    return errs.Wrap("cms.repository.get", errs.KindNotFound, err)
//	}
//
// Check the kind anywhere in the chain with [IsKind] or [KindOf].
package errs

import (
	"errors"
	"fmt"
)

// Kind categorises application errors. The value is a string because it shows up
// directly in JSON logs and RPC error payloads — readability beats iota tricks.
type Kind string

const (
	KindUnknown          Kind = "unknown"
	KindNotFound         Kind = "not_found"
	KindInvalidInput     Kind = "invalid_input"
	KindUnauthorized     Kind = "unauthorized"
	KindPermissionDenied Kind = "permission_denied"
	KindExternal         Kind = "external"
	KindConflict         Kind = "conflict"
	KindInternal         Kind = "internal"
)

// AppError is the standard error type. It implements [error] and supports
// [errors.Unwrap] / [errors.As].
type AppError struct {
	Op   string // operation identifier (e.g. "cms.repository.get"), used in logs
	Kind Kind
	Err  error // cause, wrapped with %w semantics
}

// Error returns a message of the form "op: [kind] cause".
func (e *AppError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Err == nil {
		return fmt.Sprintf("%s: [%s]", e.Op, e.Kind)
	}
	return fmt.Sprintf("%s: [%s] %v", e.Op, e.Kind, e.Err)
}

// Unwrap allows errors.Is / errors.As to traverse the chain.
func (e *AppError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Wrap returns an [AppError] with the given operation and kind, wrapping err.
// If err is nil, Wrap returns nil (convenient for one-line error returns).
func Wrap(op string, kind Kind, err error) error {
	if err == nil {
		return nil
	}
	return &AppError{Op: op, Kind: kind, Err: err}
}

// IsKind reports whether any error in err's chain is an [AppError] with the
// given kind.
func IsKind(err error, kind Kind) bool {
	if err == nil {
		return false
	}
	for cur := err; cur != nil; cur = errors.Unwrap(cur) {
		var ae *AppError
		if errors.As(cur, &ae) && ae.Kind == kind {
			return true
		}
	}
	return false
}

// KindOf returns the kind of the first [AppError] found in err's chain, or
// [KindUnknown] if none is found. Returns [KindUnknown] for nil as well.
func KindOf(err error) Kind {
	if err == nil {
		return KindUnknown
	}
	var ae *AppError
	if errors.As(err, &ae) {
		return ae.Kind
	}
	return KindUnknown
}
