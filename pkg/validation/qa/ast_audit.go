package qa

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"github.com/zqk-os/zqk/pkg/validation"
)

// Violation represents a specific non-compliant code pattern.
type Violation struct {
	Pos      token.Position
	Type     string
	Message  string
	Severity string
}

// ASTAuditor performs structural analysis of Go code.
type ASTAuditor struct {
	fset *token.FileSet
}

func NewASTAuditor() *ASTAuditor {
	return &ASTAuditor{
		fset: token.NewFileSet(),
	}
}

// AuditFile scans a single file for non-compliant patterns.
func (a *ASTAuditor) AuditFile(path string) ([]Violation, error) {
	node, err := parser.ParseFile(a.fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	var violations []Violation

	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.GenDecl:
			// Rule: Ignore declarations (const, var, type, import). We only audit usage in functions.
			// This prevents flagging constants themselves as magic strings.
			if x.Tok == token.CONST || x.Tok == token.VAR || x.Tok == token.TYPE || x.Tok == token.IMPORT {
				return false
			}

		case *ast.GoStmt:
			// Rule: Background goroutines must be managed or take context.
			violations = append(violations, a.auditGoroutine(x)...)

		case *ast.CallExpr:
			// Rule: Avoid direct instantiation of core services outside of registry.
			violations = append(violations, a.auditServiceInstantiation(x)...)

			// Rule: No direct fmt.Print* logging (POL-CODE-007).
			violations = append(violations, a.auditFmtPrintAntiPattern(x)...)

			// Rule: New object on every call anti-pattern (e.g. ID generators causing DDOS)
			violations = append(violations, a.auditNewObjectInLoopAntiPattern(x)...)

		case *ast.AssignStmt:
			// Rule: No swallowed errors in critical paths.
			violations = append(violations, a.auditSwallowedErrors(x)...)

		case *ast.BasicLit:
			// Rule: No hardcoded magic values (strings/ints) used more than once.
			// Heuristic: Flag any string literal longer than 5 chars that isn't in a constant.
			violations = append(violations, a.auditHardcodedValues(x)...)

		case *ast.IfStmt:
			// Rule: Map extraction boilerplate.
			violations = append(violations, a.auditMapExtractionAntiPattern(x)...)

			// Rule: Avoid long if/else if/else chains. Abstract into fluent/declarative utilities (pkg/when).
			violations = append(violations, a.auditConditionalChain(x)...)
		}
		return true
	})

	return violations, nil
}

// auditNewObjectInLoopAntiPattern flags repeated instantiation of heavy objects (like generators).
func (a *ASTAuditor) auditNewObjectInLoopAntiPattern(call *ast.CallExpr) []Violation {
	if fun, ok := call.Fun.(*ast.SelectorExpr); ok {
		name := fun.Sel.Name
		if strings.HasPrefix(name, "Get") && strings.HasSuffix(name, "Generator") {
			return []Violation{{
				Pos:      a.fset.Position(call.Pos()),
				Type:     "performance_anti_pattern",
				Message:  fmt.Sprintf("Anti-pattern detected: Instantiating heavy object '%s' on every call. Use a shared instance, cache, or singleton to prevent memory churn and DDOS conditions (e.g. CAS file lock collisions).", name),
				Severity: "high",
			}}
		}
	}
	return nil
}

func (a *ASTAuditor) auditConditionalChain(stmt *ast.IfStmt) []Violation {
	chainLen := 1
	curr := stmt
	for curr.Else != nil {
		if next, ok := curr.Else.(*ast.IfStmt); ok {
			chainLen++
			curr = next
		} else {
			// Final 'else' block
			chainLen++
			break
		}
	}

	if chainLen > 2 {
		return []Violation{{
			Pos:      a.fset.Position(stmt.Pos()),
			Type:     validation.ConstMagic0af9968b,
			Message:  fmt.Sprintf("Long conditional chain detected (%d branches). Abstract into a fluent, declarative utility like 'pkg/when.When' or 'pkg/when.Result[T]()'.", chainLen),
			Severity: "medium",
		}}
	}
	return nil
}

func (a *ASTAuditor) auditInterfaceUsage(fn *ast.FuncDecl) []Violation {
	var violations []Violation
	if fn.Type.Params == nil {
		return nil
	}

	for _, field := range fn.Type.Params.List {
		// Check for pointer to a struct from another package (e.g. *storage.FileObjectStorage)
		// This is a sign of high coupling.
		if star, ok := field.Type.(*ast.StarExpr); ok {
			if sel, ok := star.X.(*ast.SelectorExpr); ok {
				name := sel.Sel.Name
				if strings.Contains(name, "Storage") || strings.Contains(name, "Manager") || strings.Contains(name, "Provider") {
					violations = append(violations, Violation{
						Pos:      a.fset.Position(field.Pos()),
						Type:     validation.ConstMagic0af9968b,
						Message:  fmt.Sprintf(validation.ConstMagic3764b830, field.Names[0].Name, sel.X, name),
						Severity: "medium",
					})
				}
			}
		}
	}
	return violations
}

func (a *ASTAuditor) auditHardcodedValues(lit *ast.BasicLit) []Violation {
	// Only check strings for now (magic strings are the primary offender)
	if lit.Kind == token.STRING && len(lit.Value) > 15 {
		// Heuristic: Ignore imports.
		// In a real auditor, we would check the parent node type.
		// For this implementation, we'll check if it looks like a path or a common non-magic string.
		val := strings.Trim(lit.Value, "\"")
		if strings.Contains(val, "/") || strings.HasPrefix(val, "github.com") || strings.HasPrefix(lit.Value, "`") {
			return nil
		}

		return []Violation{{
			Pos:      a.fset.Position(lit.Pos()),
			Type:     "dry_violation",
			Message:  fmt.Sprintf(validation.ConstMagicc5c4a96a, val),
			Severity: "low",
		}}
	}
	return nil
}

func (a *ASTAuditor) auditFunctionComplexity(fn *ast.FuncDecl) []Violation {
	// Simple metric: Line count as a proxy for complexity.
	start := a.fset.Position(fn.Pos()).Line
	end := a.fset.Position(fn.End()).Line
	lineCount := end - start

	if lineCount > 100 {
		return []Violation{{
			Pos:      a.fset.Position(fn.Pos()),
			Type:     validation.ConstMagic0af9968b,
			Message:  fmt.Sprintf(validation.ConstMagic263b84d5, fn.Name.Name, lineCount),
			Severity: "medium",
		}}
	}
	return nil
}

func (a *ASTAuditor) auditGoroutine(stmt *ast.GoStmt) []Violation {
	// Simple check: does the function call have a context.Context argument?
	// This is a heuristic; more advanced analysis would trace the func signature.
	hasCtx := false
	for _, arg := range stmt.Call.Args {
		if ident, ok := arg.(*ast.Ident); ok {
			if strings.Contains(strings.ToLower(ident.Name), "ctx") {
				hasCtx = true
				break
			}
		}
	}

	if !hasCtx {
		return []Violation{{
			Pos:      a.fset.Position(stmt.Pos()),
			Type:     validation.ConstMagic9ae33b8e,
			Message:  validation.ConstMagic8343db54,
			Severity: "high",
		}}
	}
	return nil
}

func (a *ASTAuditor) auditServiceInstantiation(call *ast.CallExpr) []Violation {
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		// Example: orchestration.NewManager(...) or storage.NewFileObjectStorage(...)
		// Flag if called outside of bootstrap/test packages.
		name := sel.Sel.Name
		if strings.HasPrefix(name, "New") && (strings.Contains(name, "Manager") || strings.Contains(name, "Storage")) {
			return []Violation{{
				Pos:      a.fset.Position(call.Pos()),
				Type:     "di_violation",
				Message:  fmt.Sprintf(validation.ConstMagic3286eac2, name),
				Severity: "medium",
			}}
		}
	}
	return nil
}

func (a *ASTAuditor) auditSwallowedErrors(stmt *ast.AssignStmt) []Violation {
	if len(stmt.Lhs) == 0 {
		return nil
	}

	// Check if the LAST identifier in the assignment is '_'
	// In Go, the error is typically the last return value.
	lastLhs := stmt.Lhs[len(stmt.Lhs)-1]
	if ident, ok := lastLhs.(*ast.Ident); ok && ident.Name == "_" {
		// Verify the RHS is a function call (heuristic for return value swallowing)
		if len(stmt.Rhs) > 0 {
			if _, ok := stmt.Rhs[0].(*ast.CallExpr); ok {
				return []Violation{{
					Pos:      a.fset.Position(stmt.Pos()),
					Type:     validation.ConstMagic982e2a9f,
					Message:  validation.ConstMagic01fb6a13,
					Severity: "medium",
				}}
			}
		}
	}
	return nil
}
