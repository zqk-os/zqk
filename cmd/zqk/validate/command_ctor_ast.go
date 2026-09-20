package validate

import (
	"fmt"
	"go/ast"
	"strings"
)

func checkCommandCtorPattern(filePath string, file *ast.File) error {
	if file == nil {
		return nil
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if !isLeafCommandCtorName(fn.Name.Name) {
			continue
		}
		if wrapped := commandCtorWrapsAnother(fn); wrapped != "" {
			return fmt.Errorf("found %s wrapping %s in %s. Attach RunE to bldr_cli_cmd_v1.New*CommandBuilder() instead of mutating Use/Short on another command", fn.Name.Name, wrapped, filePath)
		}
	}
	return nil
}

func isLeafCommandCtorName(name string) bool {
	return strings.HasPrefix(name, "New") && strings.HasSuffix(name, "Cmd") && !strings.Contains(name, "Command")
}

func commandCtorWrapsAnother(fn *ast.FuncDecl) string {
	if fn == nil || fn.Body == nil {
		return ""
	}
	var wrapped string
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			for _, rhs := range x.Rhs {
				if name := wrappedCommandCtorCall(rhs); name != "" {
					wrapped = name
					return false
				}
			}
		case *ast.ReturnStmt:
			for _, res := range x.Results {
				if name := wrappedCommandCtorCall(res); name != "" {
					wrapped = name
					return false
				}
			}
		}
		return true
	})
	return wrapped
}

func wrappedCommandCtorCall(expr ast.Expr) string {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return ""
	}
	name := callExprName(call)
	if isLeafCommandCtorName(name) {
		return name
	}
	return ""
}

func callExprName(call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		return fun.Sel.Name
	default:
		return ""
	}
}
