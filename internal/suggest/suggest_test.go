package suggest

import (
	"slices"
	"testing"
)

func TestClosest(t *testing.T) {
	names := []string{"inter", "inter-tight", "inder", "lato", "roboto", "nord"}
	for _, tc := range []struct {
		name string
		n    int
		want []string
	}{
		{"Intr", 3, []string{"inter", "inder"}},
		{"inte", 3, []string{"inter", "inder", "inter-tight"}},
		{"intr", 1, []string{"inter"}},
		{"roboto-mono", 3, []string{"roboto"}},
		{"nord-dark", 3, []string{"nord"}},
		{"zzzzzz", 3, nil},
	} {
		if got := Closest(tc.name, names, tc.n); !slices.Equal(got, tc.want) {
			t.Errorf("Closest(%q, %d) = %q, want %q", tc.name, tc.n, got, tc.want)
		}
	}
}

func TestLevenshtein(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{{"", "", 0}, {"abc", "", 3}, {"", "ab", 2}, {"kitten", "sitting", 3}, {"same", "same", 0}} {
		if got := levenshtein(tc.a, tc.b); got != tc.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
