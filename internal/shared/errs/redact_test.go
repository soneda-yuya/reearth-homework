package errs_test

import (
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/soneda-yuya/overseas-safety-map/internal/shared/errs"
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

// Property: for strings of length > 8, Redact preserves exactly the first two
// and last two characters, joins them with "...", and never embeds the full
// original string.
func TestProp_RedactKeepsOnlyPrefixAndSuffix(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := rapid.StringN(9, 200, -1).Draw(t, "s")
		got := errs.Redact(s)

		// Must not contain the full original.
		if strings.Contains(got, s) {
			t.Fatalf("Redact leaked the full string: %q -> %q", s, got)
		}
		// Must have exactly the shape: first2 + "..." + last2.
		want := s[:2] + "..." + s[len(s)-2:]
		if got != want {
			t.Fatalf("Redact(%q) = %q, want %q", s, got, want)
		}
		// And the middle (indexes 2..len-2) must not appear anywhere in the
		// output — that is the actual secrecy property.
		middle := s[2 : len(s)-2]
		if len(middle) > 0 && strings.Contains(got, middle) {
			t.Fatalf("middle %q leaked into %q", middle, got)
		}
	})
}
