package io

import (
	"fmt"
	"strings"
)

// DiffClean generates a clean line-based unified diff between previous and current text.
func DiffClean(previous, current string, labelOld, labelNew string) string {
	if previous == current {
		return "No changes detected between fetches."
	}
	if labelOld == "" {
		labelOld = "previous"
	}
	if labelNew == "" {
		labelNew = "current"
	}

	prevLines := strings.Split(previous, "\n")
	currLines := strings.Split(current, "\n")

	// Standard Longest Common Subsequence (LCS) matrix
	n, m := len(prevLines), len(currLines)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}

	for i := 0; i < n; i++ {
		for j := 0; j < m; j++ {
			if prevLines[i] == currLines[j] {
				dp[i+1][j+1] = dp[i][j] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i+1][j+1] = dp[i+1][j]
			} else {
				dp[i+1][j+1] = dp[i][j+1]
			}
		}
	}

	// Backtrack to build diff ops
	type diffOp struct {
		typ  rune // ' ', '-', '+'
		line string
	}
	var ops []diffOp
	i, j := n, m
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && prevLines[i-1] == currLines[j-1] {
			ops = append(ops, diffOp{typ: ' ', line: prevLines[i-1]})
			i--
			j--
		} else if j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]) {
			ops = append(ops, diffOp{typ: '+', line: currLines[j-1]})
			j--
		} else if i > 0 && (j == 0 || dp[i][j-1] < dp[i-1][j]) {
			ops = append(ops, diffOp{typ: '-', line: prevLines[i-1]})
			i--
		}
	}

	// Reverse ops
	for l, r := 0, len(ops)-1; l < r; l, r = l+1, r-1 {
		ops[l], ops[r] = ops[r], ops[l]
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("--- %s\n", labelOld))
	sb.WriteString(fmt.Sprintf("+++ %s\n", labelNew))

	hasChanges := false
	for _, op := range ops {
		if op.typ != ' ' {
			hasChanges = true
		}
		sb.WriteString(fmt.Sprintf("%c %s\n", op.typ, op.line))
	}

	if !hasChanges {
		return "No changes detected between fetches."
	}
	return strings.TrimRight(sb.String(), "\n")
}
