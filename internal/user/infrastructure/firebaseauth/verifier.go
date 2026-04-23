// Package firebaseauth verifies Firebase ID tokens for the BFF AuthInterceptor.
//
// Verification uses firebase.google.com/go/v4/auth.Client.VerifyIDToken, which
// validates the JWT locally against Google's public keys (cached by the SDK).
// Per Code Generation Plan Q C [A] the BFF does NOT call
// VerifyIDTokenAndCheckRevoked — we trade instant revoke reflection for a
// p95-friendly codepath that never round-trips to Firebase per RPC.
package firebaseauth

import (
	"context"
	"errors"

	"firebase.google.com/go/v4/auth"

	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
)

// Verifier is the port-shaped accessor the BFF interceptor uses.
type Verifier struct {
	client TokenVerifier
}

// TokenVerifier is the subset of *auth.Client the Verifier needs. Declaring
// it as an interface lets unit tests supply a stub without a real Firebase
// project.
type TokenVerifier interface {
	VerifyIDToken(ctx context.Context, idToken string) (*auth.Token, error)
}

// New wires the Verifier to the Firebase Auth client returned by firebasex.
// The client is safe to share across goroutines.
func New(client TokenVerifier) *Verifier {
	return &Verifier{client: client}
}

// Verify validates the JWT and returns the authenticated uid. Any failure
// — missing token, expired, signature mismatch, malformed payload — surfaces
// as errs.KindUnauthorized so the ErrorInterceptor maps it to
// connect.CodeUnauthenticated.
func (v *Verifier) Verify(ctx context.Context, idToken string) (string, error) {
	if idToken == "" {
		return "", errs.Wrap("user.firebase_auth.verify", errs.KindUnauthorized,
			errors.New("empty id token"))
	}
	tok, err := v.client.VerifyIDToken(ctx, idToken)
	if err != nil {
		return "", errs.Wrap("user.firebase_auth.verify", errs.KindUnauthorized, err)
	}
	if tok == nil || tok.UID == "" {
		return "", errs.Wrap("user.firebase_auth.verify", errs.KindUnauthorized,
			errors.New("token did not yield a uid"))
	}
	return tok.UID, nil
}
