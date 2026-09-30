package qa

import (
	"fmt"
	"go/ast"

)

// auditFmtPrintAntiPattern checks for direct usage of fmt.Print, fmt.Println, fmt.Printf
// outside of allowed packages, enforcing POL-CODE-007 (All output must go through logging framework).
func (a *ASTAuditor) auditFmtPrintAntiPattern(call *ast.CallExpr) []Violation {
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "fmt" {
			// Direct print to stdout: fmt.Print, fmt.Printf, fmt.Println
			if sel.Sel.Name == "Print" || sel.Sel.Name == "Printf" || sel.Sel.Name == "Println" {
				return []Violation{{
					Pos:      a.fset.Position(call.Pos()),
					Type:     "policy_violation",
					Message:  fmt.Sprintf("POL-CODE-007 violation: direct %s call detected. Use logger.Info/Warn/Error or cli.WriteOutput.", sel.Sel.Name),
					Severity: "high",
				}}
			}

			// Stderr anti-pattern: fmt.Fprintf(os.Stderr, ...) or fmt.Fprint(os.Stderr, ...)
			if (sel.Sel.Name == "Fprintf" || sel.Sel.Name == "Fprint" || sel.Sel.Name == "Fprintln") && len(call.Args) > 0 {
				if isOsStderr(call.Args[0]) {
					return []Violation{{
						Pos:      a.fset.Position(call.Pos()),
						Type:     "policy_violation",
						Message:  fmt.Sprintf("POL-CODE-007 violation: %s(os.Stderr, ...) detected. Use structured logging (logging.Fluent) instead.", sel.Sel.Name),
						Severity: "high",
					}}
				}
			}
		}
	}
	return nil
}

// isOsStderr checks if an ast.Expr is os.Stderr.
func isOsStderr(expr ast.Expr) bool {
	if sel, ok := expr.(*ast.SelectorExpr); ok {
		if pkg, ok := sel.X.(*ast.Ident); ok {
			return pkg.Name == "os" && sel.Sel.Name == "Stderr"
		}
	}
	return false
}
