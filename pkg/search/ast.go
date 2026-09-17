package search

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// receiverTypeName extracts clean receiver type name from AST FieldList.
func receiverTypeName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	t := recv.List[0].Type
	for {
		if star, ok := t.(*ast.StarExpr); ok {
			t = star.X
		} else {
			break
		}
	}
	if ident, ok := t.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

// ASTSearchFile inspects a Go file's AST and returns matching structural declarations.
func ASTSearchFile(filePath string, data []byte, opts SearchOptions) ([]Match, error) {
	if len(data) == 0 {
		var err error
		data, err = fileutil.ReadFile(filePath)
		if err != nil {
			return nil, err
		}
	}

	fset := token.NewFileSet()
	fileNode, err := parser.ParseFile(fset, filePath, data, parser.ParseComments)
	if err != nil {
		// Non-fatal parse errors (e.g. template go files or partial syntax)
		return nil, nil
	}

	var queryRegex *regexp.Regexp
	if opts.Regex && opts.Query != "" {
		re, err := regexp.Compile(opts.Query)
		if err != nil {
			return nil, err
		}
		queryRegex = re
	}

	lines := bytes.Split(data, []byte("\n"))
	numLines := len(lines)

	var matches []Match

	matchName := func(name string) bool {
		if opts.Query == "" {
			return true
		}
		if queryRegex != nil {
			return queryRegex.MatchString(name)
		}
		if opts.CaseInsensitive {
			return strings.Contains(strings.ToLower(name), strings.ToLower(opts.Query))
		}
		return strings.Contains(name, opts.Query)
	}

	matchKind := func(kind string) bool {
		if opts.ASTKind == "" || strings.EqualFold(opts.ASTKind, "any") {
			return true
		}
		target := strings.ToLower(strings.TrimSpace(opts.ASTKind))
		kind = strings.ToLower(kind)
		if target == kind {
			return true
		}
		if target == "func" && kind == "method" {
			return true
		}
		if target == "type" && (kind == "struct" || kind == "interface") {
			return true
		}
		return false
	}

	matchReceiver := func(recv string) bool {
		if opts.ASTReceiver == "" {
			return true
		}
		target := strings.TrimPrefix(opts.ASTReceiver, "*")
		recv = strings.TrimPrefix(recv, "*")
		if opts.CaseInsensitive {
			return strings.EqualFold(recv, target)
		}
		return recv == target
	}

	for _, decl := range fileNode.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			recv := receiverTypeName(d.Recv)
			kind := "func"
			if recv != "" {
				kind = "method"
			}

			if !matchKind(kind) {
				continue
			}
			if opts.ASTReceiver != "" && (recv == "" || !matchReceiver(recv)) {
				continue
			}
			if !matchName(d.Name.Name) {
				continue
			}

			startPos := fset.Position(d.Pos())
			endPos := fset.Position(d.End())

			lineIdx := startPos.Line - 1
			lineContent := ""
			if lineIdx >= 0 && lineIdx < numLines {
				lineContent = string(lines[lineIdx])
			}

			var before []string
			if opts.ContextLines > 0 {
				from := lineIdx - opts.ContextLines
				if from < 0 {
					from = 0
				}
				for i := from; i < lineIdx; i++ {
					before = append(before, string(lines[i]))
				}
			}

			var after []string
			if opts.ContextLines > 0 {
				to := lineIdx + opts.ContextLines + 1
				if to > numLines {
					to = numLines
				}
				for i := lineIdx + 1; i < to; i++ {
					after = append(after, string(lines[i]))
				}
			}

			matches = append(matches, Match{
				File:          filePath,
				Line:          startPos.Line,
				Column:        startPos.Column,
				EndLine:       endPos.Line,
				LineContent:   lineContent,
				ContextBefore: before,
				ContextAfter:  after,
				SymbolKind:    kind,
				SymbolName:    d.Name.Name,
				Receiver:      recv,
			})

		case *ast.GenDecl:
			if opts.ASTReceiver != "" {
				continue
			}
			switch d.Tok {
			case token.TYPE:
				for _, spec := range d.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					kind := "type"
					switch ts.Type.(type) {
					case *ast.StructType:
						kind = "struct"
					case *ast.InterfaceType:
						kind = "interface"
					}

					if !matchKind(kind) {
						continue
					}
					if !matchName(ts.Name.Name) {
						continue
					}

					startPos := fset.Position(ts.Pos())
					endPos := fset.Position(ts.End())
					lineIdx := startPos.Line - 1
					lineContent := ""
					if lineIdx >= 0 && lineIdx < numLines {
						lineContent = string(lines[lineIdx])
					}

					matches = append(matches, Match{
						File:        filePath,
						Line:        startPos.Line,
						Column:      startPos.Column,
						EndLine:     endPos.Line,
						LineContent: lineContent,
						SymbolKind:  kind,
						SymbolName:  ts.Name.Name,
					})
				}

			case token.VAR, token.CONST:
				kind := "var"
				if d.Tok == token.CONST {
					kind = "const"
				}
				if !matchKind(kind) {
					continue
				}

				for _, spec := range d.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for _, nameIdent := range vs.Names {
						if !matchName(nameIdent.Name) {
							continue
						}

						startPos := fset.Position(nameIdent.Pos())
						endPos := fset.Position(vs.End())
						lineIdx := startPos.Line - 1
						lineContent := ""
						if lineIdx >= 0 && lineIdx < numLines {
							lineContent = string(lines[lineIdx])
						}

						matches = append(matches, Match{
							File:        filePath,
							Line:        startPos.Line,
							Column:      startPos.Column,
							EndLine:     endPos.Line,
							LineContent: lineContent,
							SymbolKind:  kind,
							SymbolName:  nameIdent.Name,
						})
					}
				}
			}
		}
	}

	return matches, nil
}
