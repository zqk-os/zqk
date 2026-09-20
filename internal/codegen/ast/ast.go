package ast

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// ASTInspector provides AST inspection and analysis tools segregated in internal/codegen.
type ASTInspector struct {
	Fset *token.FileSet
}

// NewASTInspector creates a new ASTInspector.
func NewASTInspector() *ASTInspector {
	return &ASTInspector{
		Fset: token.NewFileSet(),
	}
}

// ParseSource parses Go source code from bytes into an AST File.
func (ai *ASTInspector) ParseSource(filename string, src []byte) (*ast.File, error) {
	node, err := parser.ParseFile(ai.Fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, errfmt.Errorf("failed to parse Go AST source %s: %w", filename, err)
	}
	return node, nil
}

// ParseFile parses a Go file from disk into an AST File.
func (ai *ASTInspector) ParseFile(filePath string) (*ast.File, error) {
	node, err := parser.ParseFile(ai.Fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, errfmt.Errorf("failed to parse Go AST file %s: %w", filePath, err)
	}
	return node, nil
}

// FindStructNames returns all type struct names declared in an AST file.
func (ai *ASTInspector) FindStructNames(file *ast.File) []string {
	var structs []string
	ast.Inspect(file, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		if _, isStruct := ts.Type.(*ast.StructType); isStruct {
			structs = append(structs, ts.Name.Name)
		}
		return true
	})
	return structs
}

// HasEmbeddedType checks if a named struct embeds a specific type (e.g. "BaseObject", "Auditable", "Lifecycle").
func (ai *ASTInspector) HasEmbeddedType(file *ast.File, structName, embeddedTypeName string) bool {
	var found bool
	ast.Inspect(file, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok || ts.Name.Name != structName {
			return true
		}

		st, isStruct := ts.Type.(*ast.StructType)
		if !isStruct || st.Fields == nil {
			return false
		}

		for _, field := range st.Fields.List {
			if len(field.Names) == 0 { // Embedded field
				var typeName string
				switch t := field.Type.(type) {
				case *ast.Ident:
					typeName = t.Name
				case *ast.SelectorExpr:
					typeName = t.Sel.Name
				}
				if typeName == embeddedTypeName || strings.HasSuffix(typeName, "."+embeddedTypeName) {
					found = true
					return false
				}
			}
		}
		return false
	})
	return found
}
