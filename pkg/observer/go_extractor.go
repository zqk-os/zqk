package observer

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
)

const langGo = "go"

const (
	entityKindFunction  = "function"
	entityKindMethod    = "method"
	entityKindType      = "type"
	entityKindInterface = "interface"
	entityKindStruct    = "struct"
	packageMetaKey      = "package"
	signatureFuncPrefix = "func "
	goFileSuffix        = ".go"
	emptyInterfaceType  = "interface{}"
	anonymousStructType = "struct{...}"
	paramSeparator      = ", "
	emptyValue          = ""
)

// GoExtractor extracts entities from Go source using the standard library go/ast.
type GoExtractor struct{}

// Language implements Extractor.
func (GoExtractor) Language() string { return langGo }

// FileSuffix implements Extractor.
func (GoExtractor) FileSuffix() string { return goFileSuffix }

// ExtractFile parses a Go file and returns entities (functions, methods, types).
func (GoExtractor) ExtractFile(ctx context.Context, path string, content []byte) ([]Entity, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, content, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	var entities []Entity
	basePath := path
	if filepath.IsAbs(path) {
		basePath = filepath.Base(path)
	}
	pkgName := emptyValue
	if f.Name != nil {
		pkgName = f.Name.Name
	}

	var fileImports []string
	for _, imp := range f.Imports {
		if imp.Path != nil {
			fileImports = append(fileImports, strings.Trim(imp.Path.Value, `"`))
		}
	}

	ast.Inspect(f, func(n ast.Node) bool {
		if ctx.Err() != nil {
			return false
		}
		switch x := n.(type) {
		case *ast.FuncDecl:
			ent := entityFromFuncDecl(fset, basePath, pkgName, x, fileImports)
			if ent != nil {
				entities = append(entities, *ent)
			}
		case *ast.GenDecl:
			for _, spec := range x.Specs {
				if t, ok := spec.(*ast.TypeSpec); ok {
					ent := entityFromTypeSpec(fset, basePath, pkgName, t, fileImports)
					if ent != nil {
						entities = append(entities, *ent)
					}
				}
			}
		}
		return true
	})
	return entities, nil
}

func entityFromFuncDecl(fset *token.FileSet, path, pkg string, decl *ast.FuncDecl, fileImports []string) *Entity {
	kind := entityKindFunction
	receiver := emptyValue
	if decl.Recv != nil && len(decl.Recv.List) > 0 {
		kind = entityKindMethod
		receiver = typeString(decl.Recv.List[0].Type)
	}
	name := decl.Name.Name
	if name == emptyValue {
		return nil
	}
	pos := fset.Position(decl.Pos())
	var sig string
	if receiver != emptyValue {
		sig = signatureFuncPrefix + "(" + receiver + ") " + name
	} else {
		sig = signatureFuncPrefix + name
	}
	sig += " " + paramsString(decl.Type)
	meta := map[string]string{}
	if pkg != emptyValue {
		meta[packageMetaKey] = pkg
	}

	calls, dependsOn := collectDependencies(decl)
	complexity := calculateComplexity(decl)

	return &Entity{
		Kind:       kind,
		Name:       name,
		File:       path,
		Line:       pos.Line,
		Signature:  sig,
		Language:   langGo,
		Receiver:   receiver,
		Complexity: complexity,
		Calls:      calls,
		DependsOn:  dependsOn,
		Imports:    fileImports,
		Metadata:   meta,
	}
}

func calculateComplexity(node ast.Node) int {
	complexity := 1 // Base complexity
	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.CaseClause, *ast.CommClause:
			complexity++
		case *ast.BinaryExpr:
			if x.Op == token.LAND || x.Op == token.LOR {
				complexity++
			}
		}
		return true
	})
	return complexity
}

func entityFromTypeSpec(fset *token.FileSet, path, pkg string, spec *ast.TypeSpec, fileImports []string) *Entity {
	name := spec.Name.Name
	if name == emptyValue {
		return nil
	}
	kind := entityKindType
	switch spec.Type.(type) {
	case *ast.InterfaceType:
		kind = entityKindInterface
	case *ast.StructType:
		kind = entityKindStruct
	}
	pos := fset.Position(spec.Pos())
	sig := typeString(spec.Type)
	meta := map[string]string{}
	if pkg != emptyValue {
		meta[packageMetaKey] = pkg
	}

	calls, dependsOn := collectDependencies(spec.Type)

	return &Entity{
		Kind:      kind,
		Name:      name,
		File:      path,
		Line:      pos.Line,
		Signature: sig,
		Language:  langGo,
		Calls:     calls,
		DependsOn: dependsOn,
		Imports:   fileImports,
		Metadata:  meta,
	}
}

func collectDependencies(node ast.Node) ([]string, []string) {
	var calls []string
	var depends []string

	if node == nil {
		return calls, depends
	}

	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			if id, ok := x.Fun.(*ast.Ident); ok {
				calls = append(calls, id.Name)
			} else if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
				calls = append(calls, typeString(sel.X)+"."+sel.Sel.Name)
			}
		case *ast.SelectorExpr:
			depends = append(depends, typeString(x.X)+"."+x.Sel.Name)
		case *ast.Field:
			depends = append(depends, typeString(x.Type))
		}
		return true
	})

	return dedupe(calls), dedupe(depends)
}

func dedupe(s []string) []string {
	if len(s) == 0 {
		return s
	}
	m := make(map[string]bool)
	var res []string
	for _, v := range s {
		if v != "" && !m[v] {
			m[v] = true
			res = append(res, v)
		}
	}
	return res
}

func typeString(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + typeString(t.X)
	case *ast.SelectorExpr:
		return typeString(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		return "[]" + typeString(t.Elt)
	case *ast.InterfaceType:
		return emptyInterfaceType
	case *ast.StructType:
		return anonymousStructType
	case *ast.FuncType:
		return "func" + paramsString(t)
	case *ast.MapType:
		return "map[" + typeString(t.Key) + "]" + typeString(t.Value)
	default:
		return fmt.Sprintf("%T", e)
	}
}

func paramsString(ft *ast.FuncType) string {
	if ft == nil {
		return "()"
	}
	var args []string
	for _, f := range ft.Params.List {
		t := typeString(f.Type)
		if len(f.Names) > 0 {
			for range f.Names {
				args = append(args, t)
			}
		} else {
			args = append(args, t)
		}
	}
	s := "(" + strings.Join(args, paramSeparator) + ")"
	if ft.Results != nil && len(ft.Results.List) > 0 {
		var outs []string
		for _, r := range ft.Results.List {
			outs = append(outs, typeString(r.Type))
		}
		s += " (" + strings.Join(outs, paramSeparator) + ")"
	}
	return s
}
