package domain_test

import (
	"testing"

	crimemap "github.com/soneda-yuya/overseas-safety-map/internal/safetyincident/crimemap/domain"
)

func TestColorFromCount(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		count, max int
		want       string
	}{
		{"zero count is always grey", 0, 100, "#f0f0f0"},
		{"zero max is always grey", 5, 0, "#f0f0f0"},
		{"lowest bucket with count == max/5", 2, 10, "#fee5d9"},
		{"second bucket", 3, 10, "#fcae91"},
		{"middle bucket", 5, 10, "#fb6a4a"},
		{"fourth bucket", 7, 10, "#de2d26"},
		{"top bucket", 10, 10, "#a50f15"},
		{"single-spike map paints the spike hardest", 1, 1, "#a50f15"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := crimemap.ColorFromCount(tc.count, tc.max); got != tc.want {
				t.Errorf("ColorFromCount(%d, %d) = %q; want %q", tc.count, tc.max, got, tc.want)
			}
		})
	}
}
