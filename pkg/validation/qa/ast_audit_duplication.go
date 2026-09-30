package qa

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	minStatementsForFullBodyDuplication = 3
	minStatementsForSequenceDuplication = 4
	ViolationTypeDuplication            = "duplication_anti_pattern"
)

// CallableBlock represents a function declaration or function literal closure with its AST body.
type CallableBlock struct {
	Name string
	Pos  token.Position
	Body *ast.BlockStmt
}

var commentRegex = regexp.MustCompile(`//.*$|/\*[\s\S]*?\*/`)

func cleanStatementString(s string) string {
	s = commentRegex.ReplaceAllString(s, "")
	lines := strings.Split(s, "\n")
	var cleaned []string
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if t != "" {
			cleaned = append(cleaned, t)
		}
	}
	return strings.Join(cleaned, " ")
}

type statementInfo struct {
	raw   string
	lines int
}

// auditStructuralDuplication inspects functions and closures in a file for identical bodies
// or duplicated statement sequences across callable blocks.
func (a *ASTAuditor) auditStructuralDuplication(callables []CallableBlock) []Violation {
	var violations []Violation
	if len(callables) < 2 {
		return nil
	}

	type funcSig struct {
		name       string
		pos        token.Position
		statements []statementInfo
		fullBody   string
	}

	signatures := make([]funcSig, 0, len(callables))
	for _, c := range callables {
		if c.Body == nil || len(c.Body.List) == 0 {
			continue
		}

		stmts := make([]statementInfo, 0, len(c.Body.List))
		rawStmts := make([]string, 0, len(c.Body.List))
		for _, stmt := range c.Body.List {
			var buf bytes.Buffer
			// Format using a fresh FileSet to eliminate local position bias
			if err := format.Node(&buf, token.NewFileSet(), stmt); err == nil {
				formatted := cleanStatementString(buf.String())
				if formatted != "" {
					startPos := a.fset.Position(stmt.Pos())
					endPos := a.fset.Position(stmt.End())
					lines := endPos.Line - startPos.Line + 1
					if lines < 1 {
						lines = 1
					}
					stmts = append(stmts, statementInfo{
						raw:   formatted,
						lines: lines,
					})
					rawStmts = append(rawStmts, formatted)
				}
			}
		}

		if len(stmts) > 0 {
			signatures = append(signatures, funcSig{
				name:       c.Name,
				pos:        c.Pos,
				statements: stmts,
				fullBody:   strings.Join(rawStmts, "\n"),
			})
		}
	}

	reportedPairs := make(map[string]bool)

	// 1. Check for identical function / closure bodies
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
						Message:  fmt.Sprintf("Structural duplication detected: '%s' has an identical body (%d statements) to '%s'. Extract common logic into a shared helper.", f2.name, len(f2.statements), f1.name),
						Severity: "medium",
					})
				}
				continue
			}

			// 2. Check for identical statement sequences (min 3 statements OR min 2 statements spanning >= 5 lines)
			seqStmts, seqLines := findCommonSubsequence(f1.statements, f2.statements)
			if (seqStmts >= 3) || (seqStmts >= 2 && seqLines >= 5) {
				pairKey := f1.name + "::" + f2.name
				if !reportedPairs[pairKey] {
					reportedPairs[pairKey] = true
					violations = append(violations, Violation{
						Pos:      f2.pos,
						Type:     ViolationTypeDuplication,
						Message:  fmt.Sprintf("Structural duplication detected: '%s' shares a %d-statement sequence (%d lines) with '%s'. Extract duplicated sequence into a shared helper.", f2.name, seqStmts, seqLines, f1.name),
						Severity: "medium",
					})
				}
			}
		}
	}

	return violations
}

// findCommonSubsequence returns the maximum length of contiguous matching statements between s1 and s2 and the total line count.
func findCommonSubsequence(s1, s2 []statementInfo) (maxStmts int, maxLines int) {
	for i := 0; i < len(s1); i++ {
		for j := 0; j < len(s2); j++ {
			k := 0
			lines := 0
			for i+k < len(s1) && j+k < len(s2) && s1[i+k].raw == s2[j+k].raw {
				lines += s1[i+k].lines
				k++
			}
			if k > maxStmts || (k == maxStmts && lines > maxLines) {
				maxStmts = k
				maxLines = lines
			}
		}
	}
	return maxStmts, maxLines
}

// auditMapKeyReferenceDrift flags functions that test individual keys of a package-level map
// via sequential comparisons rather than looking them up directly in the map.
func (a *ASTAuditor) auditMapKeyReferenceDrift(file *ast.File) []Violation {
	if file == nil {
		return nil
	}

	constStrings := make(map[string]string)
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			valSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for j, name := range valSpec.Names {
				if j < len(valSpec.Values) {
					if lit, ok := valSpec.Values[j].(*ast.BasicLit); ok && lit.Kind == token.STRING {
						if val, err := strconv.Unquote(lit.Value); err == nil {
							constStrings[name.Name] = val
						}
					}
				}
			}
		}
	}

	type mapInfo struct {
		name string
		keys map[string]bool
	}
	var declaredMaps []mapInfo

	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			valSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, val := range valSpec.Values {
				comp, ok := val.(*ast.CompositeLit)
				if !ok {
					continue
				}
				isStringKeyMap := false
				if mapType, ok := comp.Type.(*ast.MapType); ok {
					if ident, ok := mapType.Key.(*ast.Ident); ok && ident.Name == "string" {
						isStringKeyMap = true
					}
				} else if valSpec.Type != nil {
					if mapType, ok := valSpec.Type.(*ast.MapType); ok {
						if ident, ok := mapType.Key.(*ast.Ident); ok && ident.Name == "string" {
							isStringKeyMap = true
						}
					}
				}
				if !isStringKeyMap {
					continue
				}

				var name string
				if i < len(valSpec.Names) {
					name = valSpec.Names[i].Name
				}
				if name == "" {
					continue
				}

				keys := make(map[string]bool)
				for _, elt := range comp.Elts {
					kv, ok := elt.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					switch kNode := kv.Key.(type) {
					case *ast.BasicLit:
						if kNode.Kind == token.STRING {
							if k, err := strconv.Unquote(kNode.Value); err == nil && k != "" {
								keys[k] = true
							}
						}
					case *ast.Ident:
						if val, ok := constStrings[kNode.Name]; ok && val != "" {
							keys[val] = true
						}
					}
				}

				if len(keys) >= 3 {
					declaredMaps = append(declaredMaps, mapInfo{
						name: name,
						keys: keys,
					})
				}
			}
		}
	}

	if len(declaredMaps) == 0 {
		return nil
	}

	var violations []Violation

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}

		for _, m := range declaredMaps {
			indexesMap := false
			matchedEqKeys := make(map[string]bool)
			matchedReturnKeys := make(map[string]bool)

			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if n == nil {
					return true
				}
				if id, ok := n.(*ast.Ident); ok && id.Name == m.name {
					indexesMap = true
				}
				if bin, ok := n.(*ast.BinaryExpr); ok {
					if bin.Op == token.EQL || bin.Op == token.NEQ {
						checkExpr := func(e ast.Expr) {
							switch x := e.(type) {
							case *ast.BasicLit:
								if x.Kind == token.STRING {
									if s, err := strconv.Unquote(x.Value); err == nil && m.keys[s] {
										matchedEqKeys[s] = true
									}
								}
							case *ast.Ident:
								if s, ok := constStrings[x.Name]; ok && m.keys[s] {
									matchedEqKeys[s] = true
								}
							}
						}
						checkExpr(bin.X)
						checkExpr(bin.Y)
					}
				}
				if ret, ok := n.(*ast.ReturnStmt); ok {
					for _, res := range ret.Results {
						switch x := res.(type) {
						case *ast.BasicLit:
							if x.Kind == token.STRING {
								if s, err := strconv.Unquote(x.Value); err == nil && m.keys[s] {
									matchedReturnKeys[s] = true
								}
							}
						case *ast.Ident:
							if s, ok := constStrings[x.Name]; ok && m.keys[s] {
								matchedReturnKeys[s] = true
							}
						}
					}
				}
				return true
			})

			if !indexesMap {
				if len(matchedEqKeys) >= 2 || len(matchedReturnKeys) >= 3 {
					var matched []string
					for k := range matchedEqKeys {
						matched = append(matched, k)
					}
					for k := range matchedReturnKeys {
						if !matchedEqKeys[k] {
							matched = append(matched, k)
						}
					}
					sort.Strings(matched)

					violations = append(violations, Violation{
						Pos:      a.fset.Position(fn.Pos()),
						Type:     ViolationTypeDuplication,
						Message:  fmt.Sprintf("Duplication/drift risk: function '%s' duplicates references to keys of map '%s' (%s). Use map lookup or canonical table.", fn.Name.Name, m.name, strings.Join(matched, ", ")),
						Severity: "medium",
					})
				}
			}
		}
	}

	return violations
}
