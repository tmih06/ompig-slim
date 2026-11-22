// Package editdistance provides the Levenshtein distance counted in UTF-16
// code units, as Pi's editDistance counts them. Closest-match suggestions —
// mcpext's mistyped MCP server names and the interactive mode's unknown slash
// commands — use it to judge how far a typo sits from a known name.
package editdistance

import "unicode/utf16"

// UTF16 is the Levenshtein distance between a and b in UTF-16 code units.
func UTF16(a, b string) int {
	ua, ub := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	previous := make([]int, len(ub)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(ua); i++ {
		current := make([]int, len(ub)+1)
		current[0] = i
		for j := 1; j <= len(ub); j++ {
			cost := 1
			if ua[i-1] == ub[j-1] {
				cost = 0
			}
			current[j] = min(previous[j]+1, current[j-1]+1, previous[j-1]+cost)
		}
		previous = current
	}
	return previous[len(ub)]
}
