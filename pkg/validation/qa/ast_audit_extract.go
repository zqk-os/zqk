package qa

import (
	"go/ast"

)

// auditMapExtractionAntiPattern checks for the pattern:
// if val, ok := mapVar[keyVar].(string); ok && val != emptyValue { ... }
func (a *ASTAuditor) auditMapExtractionAntiPattern(stmt *ast.IfStmt) []Violation {
	// Look for:
	// Init: AssignStmt (e.g. x, ok := m[k].(string))
	assign, ok := stmt.Init.(*ast.AssignStmt)
	if !ok || len(assign.Lhs) != 2 || len(assign.Rhs) != 1 {
		return nil
	}

	// LHS must be two identifiers (e.g., id, ok)
	_, lhs1Ok := assign.Lhs[0].(*ast.Ident)
	okIdent, lhs2Ok := assign.Lhs[1].(*ast.Ident)
	if !lhs1Ok || !lhs2Ok || okIdent.Name != "ok" {
		return nil
	}

	// RHS must be a TypeAssertExpr (e.g., m[k].(string))
	typeAssert, ok := assign.Rhs[0].(*ast.TypeAssertExpr)
	if !ok {
		return nil
	}

	// Only flag string extraction for now, since we only have GetString
	ident, ok := typeAssert.Type.(*ast.Ident)
	if !ok || ident.Name != "string" {
		return nil
	}

	// Underlying expression must be an IndexExpr (e.g., m[k])
	_, ok = typeAssert.X.(*ast.IndexExpr)
	if !ok {
		return nil
	}

	// The condition must be a BinaryExpr checking 'ok'
	binExpr, ok := stmt.Cond.(*ast.BinaryExpr)
	if !ok {
		return nil
	}

	// Just checking if 'ok' is part of the condition is enough to flag the verbose map extraction pattern
	// Let's verify 'ok' is in the condition
	isAntiPattern := false
	ast.Inspect(binExpr, func(n ast.Node) bool {
		if id, isId := n.(*ast.Ident); isId && id.Name == "ok" {
			isAntiPattern = true
			return false
		}
		return true
	})

	if isAntiPattern {
		return []Violation{{
			Pos:      a.fset.Position(stmt.Pos()),
			Type:     "abstraction_violation",
			Message:  "Verbose map extraction detected (e.g. 'if val, ok := m[k].(string); ok ...'). Abstract this boilerplate into a fluent utility like 'pkg/objects.GetString(map, key)'.",
			Severity: "medium",
		}}
	}

	return nil
}
