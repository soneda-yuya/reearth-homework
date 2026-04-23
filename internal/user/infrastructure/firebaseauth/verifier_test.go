package firebaseauth_test

import (
	"context"
	"errors"
	"testing"

	"firebase.google.com/go/v4/auth"

	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
	"github.com/soneda-yuya/overseas-safety-map/internal/user/infrastructure/firebaseauth"
)

type stubClient struct {
	tok *auth.Token
	err error
}

func (s *stubClient) VerifyIDToken(context.Context, string) (*auth.Token, error) {
	return s.tok, s.err
}

func TestVerify_OK(t *testing.T) {
	t.Parallel()
	v := firebaseauth.New(&stubClient{tok: &auth.Token{UID: "uid-1"}})
	got, err := v.Verify(context.Background(), "valid-token")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got != "uid-1" {
		t.Errorf("uid = %q; want uid-1", got)
	}
}

func TestVerify_EmptyToken(t *testing.T) {
	t.Parallel()
	v := firebaseauth.New(&stubClient{})
	if _, err := v.Verify(context.Background(), ""); !errs.IsKind(err, errs.KindUnauthorized) {
		t.Errorf("err = %v; want KindUnauthorized", err)
	}
}

func TestVerify_SDKError(t *testing.T) {
	t.Parallel()
	v := firebaseauth.New(&stubClient{err: errors.New("ID token has expired")})
	if _, err := v.Verify(context.Background(), "expired"); !errs.IsKind(err, errs.KindUnauthorized) {
		t.Errorf("err = %v; want KindUnauthorized", err)
	}
}

func TestVerify_EmptyUID(t *testing.T) {
	t.Parallel()
	// Defensive check: Firebase should never return a token without a uid,
	// but we want the failure mode to be "Unauthorized", not a nil deref
	// downstream.
	v := firebaseauth.New(&stubClient{tok: &auth.Token{UID: ""}})
	if _, err := v.Verify(context.Background(), "weird"); !errs.IsKind(err, errs.KindUnauthorized) {
		t.Errorf("err = %v; want KindUnauthorized", err)
	}
}
