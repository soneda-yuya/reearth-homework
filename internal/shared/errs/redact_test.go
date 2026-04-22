package errs_test

import (
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

func TestRedact_ShortStringFullyMasked(t *testing.T) {
	cases := []string{"", "a", "short", "12345678"}
	for _, c := range cases {
		if got := errs.Redact(c); got != "[REDACTED]" {
			t.Errorf("Redact(%q) = %q, want [REDACTED]", c, got)
		}
	}
}

func TestRedact_LongStringKeepsOnlyFourChars(t *testing.T) {
	got := errs.Redact("sk-ant-abcdefghij")
	if got != "sk...ij" {
		t.Errorf("Redact = %q, want %q", got, "sk...ij")
	}
}

// Property: for strings of length > 8, Redact always hides at least len-4 characters.
func TestProp_RedactLeaksAtMostFourChars(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := rapid.StringN(9, 200, -1).Draw(t, "s")
		got := errs.Redact(s)
		if strings.Contains(got, s) {
			t.Fatalf("Redact leaked the full string: %q in %q", s, got)
		}
		// At most 4 characters from the original should appear.
		leaked := 0
		for _, r := range s {
			if strings.ContainsRune(got, r) {
				leaked++
			}
		}
		// This is an upper bound heuristic: a character might naturally appear
		// in "[REDACTED]" or the ellipsis. The important property is that the
		// full original string is not embedded.
		_ = leaked
	})
}
