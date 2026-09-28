// Package suggest finds names close to a mistyped one, for "did you mean"
// hints.
package suggest

import (
	"slices"
	"strings"
)

// Closest returns up to n of candidates that are close to name, nearest
// first. name is lowercased; candidates are compared as given, so they should
// be lowercase too. A candidate is close when its edit distance is small for
// the length of name, or when one contains the other.
func Closest(name string, candidates []string, n int) []string {
	type candidate struct {
		s    string
		dist int
	}
	name = strings.ToLower(name)
	maxDist := max(2, len(name)/3)
	var found []candidate
	for _, c := range candidates {
		d := levenshtein(name, c)
		if d <= maxDist || strings.Contains(c, name) || (len(c) >= 3 && strings.Contains(name, c)) {
			found = append(found, candidate{c, d})
		}
	}
	slices.SortStableFunc(found, func(a, b candidate) int { return a.dist - b.dist })
	var out []string
	for _, c := range found[:min(n, len(found))] {
		out = append(out, c.s)
	}
	return out
}

func levenshtein(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}
