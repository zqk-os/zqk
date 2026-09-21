// Flags user-facing "zqk <verb>" string literals in production Go function bodies.
// Use pkg/paths.CLIUsage / CLIInvocation / RewriteCanonicalCLIInvocations instead.
//
//	go run ./scripts/check_cli_name_literals [repo-root] [-write] [file.go...]
package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/tools/go/ast/astutil"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

var exemptFuncs = map[string]struct{}{
	"CLIUsage":                       {},
	"CLIInvocation":                  {},
	"RewriteCanonicalCLIInvocations": {},
}

func main() {
	repo := "."
	write := false
	var positionals []string
	for _, arg := range os.Args[1:] {
		switch arg {
		case "-write", "--write", "-fix", "--fix":
			write = true
		default:
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintf(os.Stderr, "unknown flag: %s\n", arg)
				os.Exit(2)
			}
			positionals = append(positionals, arg)
		}
	}
	var extra []string
	if len(positionals) > 0 {
		if fi, err := os.Stat(positionals[0]); err == nil && fi.IsDir() {
			repo = positionals[0]
			extra = positionals[1:]
		} else {
			extra = positionals
		}
	}
	brand.SetExecutableName(brand.CanonicalExecutableToken)

	var files []string
	var err error
	if len(extra) > 0 {
		files = extra
	} else {
		files, err = gitGoFiles(repo)
		if err != nil {
			fmt.Fprintf(os.Stderr, "check_cli_name_literals: %v\n", err)
			os.Exit(2)
		}
	}

	var violations []string
	for _, rel := range files {
		if shouldSkip(rel) {
			continue
		}
		abs := rel
		if !filepath.IsAbs(rel) {
			abs = filepath.Join(repo, rel)
		}
		src, err := fileutil.ReadFile(abs)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			fmt.Fprintf(os.Stderr, "check_cli_name_literals: read %s: %v\n", rel, err)
			os.Exit(2)
		}
		if isGenerated(src) {
			continue
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, rel, src, parser.ParseComments)
		if err != nil {
			fmt.Fprintf(os.Stderr, "check_cli_name_literals: parse %s: %v\n", rel, err)
			os.Exit(2)
		}
		wrap := map[token.Pos]bool{}
		var stack []ast.Node
		ast.Inspect(f, func(n ast.Node) bool {
			if n == nil {
				if len(stack) > 0 {
					stack = stack[:len(stack)-1]
				}
				return false
			}
			stack = append(stack, n)
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			if callExempt(stack) || inConstDecl(stack) {
				return true
			}
			s, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			if hit, verb := firstCanonicalCLIHit(s); hit {
				pos := fset.Position(lit.Pos())
				violations = append(violations, fmt.Sprintf("%s:%d: hardcoded %q %s invocation — use pkg/paths.CLIUsage / CLIInvocation / RewriteCanonicalCLIInvocations", rel, pos.Line, brand.CanonicalExecutableToken, verb))
				wrap[lit.Pos()] = true
			}
			return true
		})
		if write {
			fileChanged := false
			if len(wrap) > 0 {
				astutil.Apply(f, func(c *astutil.Cursor) bool {
					lit, ok := c.Node().(*ast.BasicLit)
					if !ok || !wrap[lit.Pos()] {
						return true
					}
					c.Replace(&ast.CallExpr{
						Fun: &ast.SelectorExpr{
							X:   ast.NewIdent("paths"),
							Sel: ast.NewIdent("RewriteCanonicalCLIInvocations"),
						},
						Args: []ast.Expr{lit},
					})
					return true
				}, nil)
				astutil.AddNamedImport(fset, f, "", "github.com/zqk-os/zqk/pkg/paths")
				fileChanged = true
			}
			printfChanged, needsFmt := rewritePrintfRewriteCalls(f)
			if needsFmt {
				astutil.AddNamedImport(fset, f, "", "fmt")
			}
			if printfChanged {
				fileChanged = true
			}
			if fileChanged {
				var buf bytes.Buffer
				if err := format.Node(&buf, fset, f); err != nil {
					fmt.Fprintf(os.Stderr, "check_cli_name_literals: format %s: %v\n", rel, err)
					os.Exit(2)
				}
				if err := fileutil.WriteFile(abs, buf.Bytes(), paths.FilePerm644); err != nil {
					fmt.Fprintf(os.Stderr, "check_cli_name_literals: write %s: %v\n", rel, err)
					os.Exit(2)
				}
			}
		}
	}
	if write {
		fmt.Fprintf(os.Stdout, "check_cli_name_literals: wrapped %d literal(s)\n", len(violations))
		return
	}
	if len(violations) > 0 {
		for _, v := range violations {
			fmt.Fprintln(os.Stdout, v)
		}
		fmt.Fprintf(os.Stderr, "\ncheck_cli_name_literals: %d violation(s).\n", len(violations))
		os.Exit(1)
	}
	fmt.Fprintln(os.Stdout, "check_cli_name_literals: ok (no hardcoded canonical CLI invocations in production function bodies)")
}

func firstCanonicalCLIHit(s string) (bool, string) {
	needle := brand.CanonicalExecutableToken + " "
	rest := s
	for {
		i := strings.Index(rest, needle)
		if i < 0 {
			return false, ""
		}
		if i > 0 {
			prev := rune(rest[i-1])
			if unicode.IsLetter(prev) || unicode.IsDigit(prev) || prev == '_' || prev == '-' {
				rest = rest[i+len(needle):]
				continue
			}
		}
		after := rest[i+len(needle):]
		verb, _, _ := strings.Cut(after, " ")
		verb = strings.TrimRight(verb, "\"'`.,;:)")
		if paths.IsProductCLIVerb(verb) {
			return true, verb
		}
		rest = after
	}
}

func callExempt(stack []ast.Node) bool {
	for i := len(stack) - 1; i >= 0; i-- {
		call, ok := stack[i].(*ast.CallExpr)
		if !ok {
			continue
		}
		name := callFunName(call)
		if _, ok := exemptFuncs[name]; ok {
			return true
		}
	}
	return false
}

func inConstDecl(stack []ast.Node) bool {
	for _, n := range stack {
		if gd, ok := n.(*ast.GenDecl); ok && gd.Tok == token.CONST {
			// Package-level const templates still author the canonical executable token;
			// rewrite at use or convert to functions so white-label settings applied after
			// init are honored.
			return true
		}
	}
	return false
}

func callFunName(call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		if fun.Sel != nil {
			return fun.Sel.Name
		}
	}
	return ""
}

func rewritePrintfRewriteCalls(f *ast.File) (changed, needsFmt bool) {
	astutil.Apply(f, func(c *astutil.Cursor) bool {
		call, ok := c.Node().(*ast.CallExpr)
		if !ok {
			return true
		}
		name := callFunName(call)
		formatIdx := 0
		switch name {
		case "Fprintf":
			formatIdx = 1
		case "Errorf", "Newf", "Sprintf", "Printf":
			formatIdx = 0
		default:
			return true
		}
		if len(call.Args) <= formatIdx {
			return true
		}
		inner, ok := call.Args[formatIdx].(*ast.CallExpr)
		if !ok || callFunName(inner) != "RewriteCanonicalCLIInvocations" || len(inner.Args) != 1 {
			return true
		}
		rest := append([]ast.Expr(nil), call.Args[formatIdx+1:]...)
		origFmt := inner.Args[0]
		var branded ast.Expr
		if len(rest) == 0 {
			branded = inner
		} else {
			needsFmt = true
			branded = &ast.CallExpr{
				Fun: inner.Fun,
				Args: []ast.Expr{&ast.CallExpr{
					Fun:  &ast.SelectorExpr{X: ast.NewIdent("fmt"), Sel: ast.NewIdent("Sprintf")},
					Args: append([]ast.Expr{origFmt}, rest...),
				}},
			}
		}
		if name == "Sprintf" {
			c.Replace(branded)
			changed = true
			return true
		}
		newArgs := append([]ast.Expr(nil), call.Args[:formatIdx]...)
		newArgs = append(newArgs, &ast.BasicLit{Kind: token.STRING, Value: `"%s"`}, branded)
		c.Replace(&ast.CallExpr{Fun: call.Fun, Args: newArgs})
		changed = true
		return true
	}, nil)
	return changed, needsFmt
}

func shouldSkip(rel string) bool {
	rel = filepath.ToSlash(rel)
	switch {
	case strings.HasSuffix(rel, "_test.go"):
		return true
	case strings.Contains(rel, "/testdata/"):
		return true
	case strings.HasPrefix(rel, "vendor/"):
		return true
	case rel == "pkg/paths/cli_command_name.go":
		return true
	case strings.HasPrefix(rel, "scripts/check_cli_name_literals/"):
		return true
	}
	return false
}

func isGenerated(src []byte) bool {
	head := string(src)
	if len(head) > 512 {
		head = head[:512]
	}
	return strings.Contains(head, "Code generated") && strings.Contains(head, "DO NOT EDIT")
}

func gitGoFiles(repo string) ([]string, error) {
	cmd := execwrap.Command("git", "-C", repo, "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", "*.go")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var files []string
	for _, p := range strings.Split(string(out), "\x00") {
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, "pkg/") || strings.HasPrefix(p, "cmd/") || strings.HasPrefix(p, "internal/") || strings.HasPrefix(p, "scripts/") {
			files = append(files, p)
		}
	}
	return files, nil
}
