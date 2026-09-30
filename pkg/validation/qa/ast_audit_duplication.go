package qa

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"strings"
)

const (
	minStatementsForFullBodyDuplication = 3
	minStatementsForSequenceDuplication = 4
	ViolationTypeDuplication            = "duplication_anti_pattern"
)

// auditStructuralDuplication inspects functions in a file for identical function bodies
// or duplicated statement sequences across functions.
func (a *ASTAuditor) auditStructuralDuplication(funcs []*ast.FuncDecl) []Violation {
	var violations []Violation
	if len(funcs) < 2 {
		return nil
	}

	type funcSig struct {
		name       string
		pos        token.Position
		statements []string
		fullBody   string
	}

	signatures := make([]funcSig, 0, len(funcs))
	for _, fn := range funcs {
		if fn == nil || fn.Body == nil || len(fn.Body.List) == 0 {
			continue
		}

		stmts := make([]string, 0, len(fn.Body.List))
		for _, stmt := range fn.Body.List {
			var buf bytes.Buffer
			// Format using a fresh FileSet to eliminate local position bias
			if err := format.Node(&buf, token.NewFileSet(), stmt); err == nil {
				formatted := strings.TrimSpace(buf.String())
				if formatted != "" {
					stmts = append(stmts, formatted)
				}
			}
		}

		if len(stmts) > 0 {
			fnName := ""
			if fn.Name != nil {
				fnName = fn.Name.Name
			}
			signatures = append(signatures, funcSig{
				name:       fnName,
				pos:        a.fset.Position(fn.Pos()),
				statements: stmts,
				fullBody:   strings.Join(stmts, "\n"),
			})
		}
	}

	reportedPairs := make(map[string]bool)

	// 1. Check for identical function bodies
	for i := 0; i < len(signatures); i++ {
		for j := i + 1; j < len(signatures); j++ {
			f1 := signatures[i]
			f2 := signatures[j]

			if len(f1.statements) >= minStatementsForFullBodyDuplication &&
				f1.fullBody == f2.fullBody {
				pairKey := f1.name + "::" + f2.name
				if !reportedPairs[pairKey] {
					reportedPairs[pairKey] = true
					violations = append(violations, Violation{
						Pos:      f2.pos,
						Type:     ViolationTypeDuplication,
						Message:  fmt.Sprintf("Structural duplication detected: function '%s' has an identical body (%d statements) to '%s'. Extract common logic into a shared helper.", f2.name, len(f2.statements), f1.name),
						Severity: "medium",
					})
				}
				continue
			}

			// 2. Check for identical statement sequences (min 4 statements)
			if len(f1.statements) >= minStatementsForSequenceDuplication &&
				len(f2.statements) >= minStatementsForSequenceDuplication {
				seqLen := findCommonSubsequenceLength(f1.statements, f2.statements)
				if seqLen >= minStatementsForSequenceDuplication {
					pairKey := f1.name + "::" + f2.name
					if !reportedPairs[pairKey] {
						reportedPairs[pairKey] = true
						violations = append(violations, Violation{
							Pos:      f2.pos,
							Type:     ViolationTypeDuplication,
							Message:  fmt.Sprintf("Structural duplication detected: function '%s' shares a %d-statement sequence with '%s'. Extract duplicated sequence into a shared helper.", f2.name, seqLen, f1.name),
							Severity: "medium",
						})
					}
				}
			}
		}
	}

	return violations
}

// findCommonSubsequenceLength returns the maximum length of contiguous matching statements between s1 and s2.
func findCommonSubsequenceLength(s1, s2 []string) int {
	maxLen := 0
	for i := 0; i < len(s1); i++ {
		for j := 0; j < len(s2); j++ {
			k := 0
			for i+k < len(s1) && j+k < len(s2) && s1[i+k] == s2[j+k] {
				k++
			}
			if k > maxLen {
				maxLen = k
			}
		}
	}
	return maxLen
}
