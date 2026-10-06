package strutil

import "math"

const maxLevenshteinStringLen = 4096

// LevenshteinDistance calculates the Levenshtein distance between two strings.
func LevenshteinDistance(s1, s2 string) int {
	la, lb := len(s1), len(s2)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	if la > maxLevenshteinStringLen || lb > maxLevenshteinStringLen {
		if la > lb {
			return la
		}
		return lb
	}

	// Swap so s2 is always the shorter string for O(min(la, lb)) space
	if la < lb {
		s1, s2 = s2, s1
		la, lb = lb, la
	}

	cols := lb
	if cols < math.MaxInt {
		cols++
	}

	prev := make([]int, cols)
	curr := make([]int, cols)
	for j := 0; j < cols; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j < cols; j++ {
			cost := 1
			if s1[i-1] == s2[j-1] {
				cost = 0
			}
			curr[j] = min(prev[j]+1, min(curr[j-1]+1, prev[j-1]+cost))
		}
		copy(prev, curr)
	}
	return prev[lb]
}
