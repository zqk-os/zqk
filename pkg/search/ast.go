package search

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
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

// ASTSearchFile inspects a supported source file (Go, TypeScript/JS, Python, Rust)
// and returns matching structural declarations.
func ASTSearchFile(filePath string, data []byte, opts SearchOptions) ([]Match, error) {
	if len(data) == 0 {
		var err error
		data, err = fileutil.ReadFile(filePath)
		if err != nil {
			return nil, err
		}
	}

	if ext := filepath.Ext(filePath); isPolyglotASTExt(ext) {
		return polyglotASTSearchFile(filePath, data, opts)
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
		return matchASTName(opts.Query, name, queryRegex, opts.CaseInsensitive)
	}

	matchKind := func(kind string) bool {
		return matchASTKind(opts.ASTKind, kind)
	}

	matchReceiver := func(recv string) bool {
		return matchASTReceiver(opts.ASTReceiver, recv, opts.CaseInsensitive)
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

			before, after := collectASTContext(lines, lineIdx, opts.ContextLines)

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
