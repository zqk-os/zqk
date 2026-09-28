// scripts/check_path_and_perm_literals/main.go
//
// AST checker and fixer for:
// 1. Hardcoded path references (".zqk", ".zqk/...", filepath.Join with literal segments).
// 2. Hardcoded magic file permissions (0755, 0644, 0600, 0700, 0750 in file operations).
// 3. Duplicate string literals (used >= 2 times outside of constants).
//
// Usage:
//
//	go run ./scripts/check_path_and_perm_literals [repo-root] [flags] [files...]
//
// Flags:
//
//	--write / --fix    Automatically rewrite AST and apply fixes to files.
//	--no-dups          Skip duplicate string literals scan.
//	--no-paths         Skip hardcoded paths scan.
//	--no-perms         Skip magic file permissions scan.
//	--quiet            Suppress heartbeat output.
package main

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/tools/go/ast/astutil"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

const (
	progressEveryN = 75
	heartbeatEvery = 25 * time.Second
)

type Violation struct {
	Pos     token.Position
	Kind    string // "PATH", "PERM", "DUP"
	Message string
}

func main() {
	repo := "."
	var files []string
	autoFix := false
	checkPaths := true
	checkPerms := true
	checkDups := true
	quiet := os.Getenv("CHECK_AST_QUIET") != ""

	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--write", "--fix", "-write", "-fix":
			autoFix = true
		case "--no-dups":
			checkDups = false
		case "--no-paths":
			checkPaths = false
		case "--no-perms":
			checkPerms = false
		case "--quiet", "-q":
			quiet = true
		default:
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintf(os.Stderr, "unknown flag: %s\n", arg)
				os.Exit(2)
			}
			fi, err := os.Stat(arg)
			if repo == "." && i == 0 && err == nil && fi.IsDir() {
				repo = arg
				continue
			}
			if err == nil && fi.IsDir() {
				_ = filepath.Walk(arg, func(path string, info os.FileInfo, walkErr error) error {
					if walkErr == nil && !info.IsDir() && strings.HasSuffix(path, ".go") {
						files = append(files, path)
					}
					return nil
				})
			} else {
				files = append(files, arg)
			}
		}
	}

	repoAbs, err := filepath.Abs(repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve repo root: %v\n", err)
		os.Exit(2)
	}

	pathsImport, err := detectPathsImport(repoAbs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "detect module paths: %v\n", err)
		os.Exit(2)
	}

	if len(files) == 0 {
		files, err = gitGoFiles(repoAbs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "list go files: %v\n", err)
			os.Exit(2)
		}
	}

	if !quiet {
		fmt.Fprintf(os.Stderr, "check_path_and_perm_literals: %d candidate Go files (write=%v paths=%v perms=%v dups=%v)\n",
			len(files), autoFix, checkPaths, checkPerms, checkDups)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var scanIndex atomic.Int64
	var scanPath atomic.Value
	scanIndex.Store(-1)
	if !quiet {
		goroutinelabels.NewGoroutine("heartbeat", "heartbeat progress reporter").
			WithContext(ctx).
			StartSimple(func() {
				heartbeat(ctx, len(files), heartbeatEvery, &scanIndex, &scanPath)
			})
	}

	var allViolations []Violation
	fset := token.NewFileSet()
	fixedFilesCount := 0

	for i, rel := range files {
		scanIndex.Store(int64(i))
		scanPath.Store(rel)

		if shouldSkipFile(rel) {
			continue
		}

		abs := filepath.Join(repoAbs, rel)
		src, err := os.ReadFile(abs)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			fmt.Fprintf(os.Stderr, "read %s: %v\n", rel, err)
			os.Exit(2)
		}

		f, err := parser.ParseFile(fset, rel, src, parser.ParseComments)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse %s: %v\n", rel, err)
			os.Exit(2)
		}

		modified := false

		// 1. Path checking and fixing
		if checkPaths {
			v, mod := inspectPaths(fset, f, rel, autoFix, pathsImport)
			allViolations = append(allViolations, v...)
			if mod {
				modified = true
			}
		}

		// 2. Permission checking and fixing
		if checkPerms {
			v, mod := inspectPermissions(fset, f, rel, autoFix, pathsImport)
			allViolations = append(allViolations, v...)
			if mod {
				modified = true
			}
		}

		// 3. Duplicate string literals checking
		if checkDups {
			v := inspectDuplicateStrings(fset, f, rel)
			allViolations = append(allViolations, v...)
		}

		if autoFix && modified {
			var buf bytes.Buffer
			if err := format.Node(&buf, fset, f); err != nil {
				fmt.Fprintf(os.Stderr, "format %s: %v\n", rel, err)
				os.Exit(2)
			}
			if err := os.WriteFile(abs, buf.Bytes(), 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "write %s: %v\n", rel, err)
				os.Exit(2)
			}
			fixedFilesCount++
		}
	}

	if autoFix && fixedFilesCount > 0 {
		fmt.Printf("✅ Automatically applied AST fixes to %d files\n", fixedFilesCount)
	}

	if len(allViolations) > 0 {
		// Group by kind for clear summary
		pathCount := 0
		permCount := 0
		dupCount := 0
		for _, v := range allViolations {
			switch v.Kind {
			case "PATH":
				pathCount++
			case "PERM":
				permCount++
			case "DUP":
				dupCount++
			}
			fmt.Printf("%s:%d:%d: [%s] %s\n", v.Pos.Filename, v.Pos.Line, v.Pos.Column, v.Kind, v.Message)
		}

		fmt.Fprintf(os.Stderr, "\nFound %d violations (paths: %d, permissions: %d, duplicate literals: %d).\n",
			len(allViolations), pathCount, permCount, dupCount)
		if !autoFix && (pathCount > 0 || permCount > 0) {
			fmt.Fprintf(os.Stderr, "Run with --write or --fix to automatically resolve path and permission violations.\n")
		}
		if !autoFix {
			os.Exit(1)
		}
	} else {
		fmt.Println("✅ No path, permission, or duplicate literal violations found.")
	}
}

func shouldSkipFile(rel string) bool {
	// Skip vendor, build artifacts, git, and path definitions themselves
	switch {
	case strings.HasPrefix(rel, "vendor/"):
		return true
	case strings.HasPrefix(rel, ".git/"):
		return true
	case strings.HasPrefix(rel, "pkg/paths/"):
		return true
	case strings.HasPrefix(rel, "pkg/brand/"):
		return true
	case strings.HasPrefix(rel, "scripts/check_path_and_perm_literals/"):
		return true
	case strings.HasPrefix(rel, "pkg/specbuilder/bldr_"):
		return true
	case strings.HasPrefix(rel, "pkg/cli/bldr_cli_cmd_v1/"):
		return true
	case strings.HasPrefix(rel, "pkg/utils/fileutil/"):
		return true
	case strings.HasPrefix(rel, "scripts/prepare_cas_updates/"):
		return true
	case strings.HasPrefix(rel, "scripts/legacy-decommission/"):
		return true
	case strings.Contains(rel, "tools_sandbox"):
		return true
	case strings.HasSuffix(rel, "test_bundle_rerun_test.go"):
		return true
	}
	return false
}

// ----------------------------------------------------------------------------
// Path Inspections & Fixes
// ----------------------------------------------------------------------------

var knownSubdirConstants = map[string]string{
	"process":       "ProcessSubdir",
	"specs":         "SpecsSubdir",
	"cache":         "CacheDir",
	"logs":          "LogsDir",
	"config":        "ConfigDir",
	"agent-runtime": "AgentRuntimeDir",
	"state":         "StateDir",
	"wal":           "WalDir",
	"metrics":       "MetricsDir",
	"scheduler":     "SchedulerSubdir",
	"skills":        "SkillsSubdir",
	"ides":          "IdesSubdir",
	"inbox":         "InboxSubdir",
	"streams":       "StreamsSubdir",
	"worktrees":     "WorktreesSubdir",
	"object_drafts": "ObjectDraftsDir",
	"drafts":        "DraftsSubdir",
	"cleanup":       "CleanupSubdir",
}

func inspectPaths(fset *token.FileSet, f *ast.File, rel string, autoFix bool, pathsImport string) ([]Violation, bool) {
	var violations []Violation
	modified := false

	astutil.Apply(f, func(cursor *astutil.Cursor) bool {
		n := cursor.Node()

		// 1. Check filepath.Join or path.Join
		if call, ok := n.(*ast.CallExpr); ok && isPathJoinCall(call) {
			for i, arg := range call.Args {
				lit, ok := arg.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				val, err := strconv.Unquote(lit.Value)
				if err != nil {
					val = lit.Value
				}

				// Check for hardcoded ".zqk-state"
				if val == ".zqk-state" {
					violations = append(violations, Violation{
						Pos:     fset.Position(lit.Pos()),
						Kind:    "PATH",
						Message: `hardcoded ".zqk-state" in path join — use paths.ProjectStateDir`,
					})
					if autoFix {
						call.Args[i] = &ast.SelectorExpr{
							X:   ast.NewIdent("paths"),
							Sel: ast.NewIdent("ProjectStateDir"),
						}
						astutil.AddImport(fset, f, pathsImport)
						modified = true
					}
					continue
				}

				// Check for hardcoded ".zqk"
				if val == ".zqk" {
					violations = append(violations, Violation{
						Pos:     fset.Position(lit.Pos()),
						Kind:    "PATH",
						Message: `hardcoded ".zqk" in path join — use paths.ProjectDataDir`,
					})
					if autoFix {
						call.Args[i] = &ast.SelectorExpr{
							X:   ast.NewIdent("paths"),
							Sel: ast.NewIdent("ProjectDataDir"),
						}
						astutil.AddImport(fset, f, pathsImport)
						modified = true
					}
					continue
				}

				// Check for ".zqk/something"
				if strings.HasPrefix(val, ".zqk/") {
					violations = append(violations, Violation{
						Pos:     fset.Position(lit.Pos()),
						Kind:    "PATH",
						Message: fmt.Sprintf(`hardcoded ".zqk" path prefix %q in path join — use paths constants`, val),
					})
					if autoFix {
						// Split by slash and construct join
						sub := strings.TrimPrefix(val, ".zqk/")
						if constName, exists := knownSubdirConstants[sub]; exists {
							call.Args[i] = &ast.SelectorExpr{
								X:   ast.NewIdent("paths"),
								Sel: ast.NewIdent(constName),
							}
							// Prepend paths.ProjectDataDir before this arg
							newArgs := make([]ast.Expr, 0, len(call.Args)+1)
							newArgs = append(newArgs, call.Args[:i]...)
							newArgs = append(newArgs, &ast.SelectorExpr{
								X:   ast.NewIdent("paths"),
								Sel: ast.NewIdent("ProjectDataDir"),
							})
							newArgs = append(newArgs, call.Args[i:]...)
							call.Args = newArgs
						} else {
							call.Args[i] = &ast.SelectorExpr{
								X:   ast.NewIdent("paths"),
								Sel: ast.NewIdent("ProjectDataDir"),
							}
							call.Args = append(call.Args, &ast.BasicLit{
								Kind:  token.STRING,
								Value: strconv.Quote(sub),
							})
						}
						astutil.AddImport(fset, f, pathsImport)
						modified = true
					}
					continue
				}

				// Check for hardcoded subdirectories when following a ProjectDataDir or root
				if constName, exists := knownSubdirConstants[val]; exists {
					// If previous arg was paths.ProjectDataDir or ".zqk"
					prevIsZqk := false
					if i > 0 {
						if prevSel, ok := call.Args[i-1].(*ast.SelectorExpr); ok {
							if prevId, ok := prevSel.X.(*ast.Ident); ok && prevId.Name == "paths" && prevSel.Sel.Name == "ProjectDataDir" {
								prevIsZqk = true
							}
						}
					}
					if prevIsZqk {
						violations = append(violations, Violation{
							Pos:     fset.Position(lit.Pos()),
							Kind:    "PATH",
							Message: fmt.Sprintf(`hardcoded path segment %q — use paths.%s`, val, constName),
						})
						if autoFix {
							call.Args[i] = &ast.SelectorExpr{
								X:   ast.NewIdent("paths"),
								Sel: ast.NewIdent(constName),
							}
							astutil.AddImport(fset, f, pathsImport)
							modified = true
						}
					}
				}
			}
		}

		// 2. Raw string literal containing ".zqk/" or ".zqk-state/"
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			val, _ := strconv.Unquote(lit.Value)
			// Ignore long templates / multi-line bash scripts
			if strings.Count(val, "\n") <= 5 {
				if strings.HasPrefix(val, ".zqk/") || strings.Contains(val, "/.zqk/") {
					violations = append(violations, Violation{
						Pos:     fset.Position(lit.Pos()),
						Kind:    "PATH",
						Message: fmt.Sprintf(`raw path literal %q contains .zqk — use paths constants`, val),
					})
				} else if strings.HasPrefix(val, ".zqk-state/") || strings.Contains(val, "/.zqk-state/") || val == ".zqk-state" {
					violations = append(violations, Violation{
						Pos:     fset.Position(lit.Pos()),
						Kind:    "PATH",
						Message: fmt.Sprintf(`raw path literal %q contains .zqk-state — use paths constants`, val),
					})
				}
			}
		}

		return true
	}, nil)

	return violations, modified
}

func isPathJoinCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return (id.Name == "filepath" || id.Name == "path") && sel.Sel.Name == "Join"
}

// ----------------------------------------------------------------------------
// Permission Inspections & Fixes
// ----------------------------------------------------------------------------

func inspectPermissions(fset *token.FileSet, f *ast.File, rel string, autoFix bool, pathsImport string) ([]Violation, bool) {
	var violations []Violation
	modified := false

	astutil.Apply(f, func(cursor *astutil.Cursor) bool {
		n := cursor.Node()

		// 1. Variable and constant declarations with raw octal permissions
		if vs, ok := n.(*ast.ValueSpec); ok {
			for i, val := range vs.Values {
				lit, ok := val.(*ast.BasicLit)
				if !ok || lit.Kind != token.INT {
					continue
				}
				permStr := lit.Value
				constName := resolvePermConstant(permStr, false)
				if constName == "" {
					continue
				}
				isMode := false
				if vs.Type != nil {
					typeStr := fmt.Sprintf("%v", vs.Type)
					if strings.Contains(typeStr, "FileMode") || strings.Contains(typeStr, "Mode") {
						isMode = true
					}
				}
				for _, name := range vs.Names {
					nLower := strings.ToLower(name.Name)
					if strings.Contains(nLower, "perm") || strings.Contains(nLower, "mode") {
						isMode = true
						break
					}
				}
				if isMode {
					violations = append(violations, Violation{
						Pos:     fset.Position(lit.Pos()),
						Kind:    "PERM",
						Message: fmt.Sprintf("magic file permission %s in declaration — use paths.%s", permStr, constName),
					})
					if autoFix {
						vs.Values[i] = &ast.SelectorExpr{
							X:   ast.NewIdent("paths"),
							Sel: ast.NewIdent(constName),
						}
						astutil.AddImport(fset, f, pathsImport)
						modified = true
					}
				}
			}
		}

		// 2. Function calls with raw octal permissions
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		permArgIdx, isFileOp, isExecutable := getFilePermArgIndex(call)
		if !isFileOp || permArgIdx >= len(call.Args) {
			return true
		}

		arg := call.Args[permArgIdx]
		lit, ok := arg.(*ast.BasicLit)
		if !ok || lit.Kind != token.INT {
			return true
		}

		permStr := lit.Value
		constName := resolvePermConstant(permStr, isExecutable)
		if constName == "" {
			return true
		}

		violations = append(violations, Violation{
			Pos:     fset.Position(lit.Pos()),
			Kind:    "PERM",
			Message: fmt.Sprintf("magic file permission %s — use paths.%s", permStr, constName),
		})

		if autoFix {
			call.Args[permArgIdx] = &ast.SelectorExpr{
				X:   ast.NewIdent("paths"),
				Sel: ast.NewIdent(constName),
			}
			astutil.AddImport(fset, f, pathsImport)
			modified = true
		}

		return true
	}, nil)

	return violations, modified
}

func getFilePermArgIndex(call *ast.CallExpr) (idx int, isFileOp bool, isExec bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil {
		return -1, false, false
	}
	pkgIdent, ok := sel.X.(*ast.Ident)
	if !ok {
		return -1, false, false
	}

	pkgName := pkgIdent.Name
	funcName := sel.Sel.Name

	if pkgName != "os" && pkgName != "fileutil" {
		return -1, false, false
	}

	switch funcName {
	case "WriteFile":
		return 2, true, false
	case "Mkdir", "MkdirAll":
		return 1, true, false
	case "OpenFile":
		return 2, true, false
	case "Chmod":
		return 1, true, false
	default:
		return -1, false, false
	}
}

func resolvePermConstant(permStr string, isExec bool) string {
	clean := strings.TrimPrefix(permStr, "0o")
	clean = strings.TrimPrefix(clean, "0O")
	clean = strings.TrimPrefix(clean, "0")

	switch clean {
	case "755":
		if isExec {
			return "FilePerm755"
		}
		return "DirPerm755"
	case "644":
		return "FilePerm644"
	case "600":
		return "FilePerm600"
	case "700":
		return "DirPerm700"
	case "750":
		return "DirPerm750"
	default:
		return ""
	}
}

// ----------------------------------------------------------------------------
// Duplicate String Literal Inspections
// ----------------------------------------------------------------------------

func inspectDuplicateStrings(fset *token.FileSet, f *ast.File, rel string) []Violation {
	var violations []Violation

	// Count non-trivial string literals
	counts := make(map[string][]token.Pos)

	ast.Inspect(f, func(n ast.Node) bool {
		// Skip import specs
		if _, ok := n.(*ast.ImportSpec); ok {
			return false
		}
		// Skip const decls (they are already constants!)
		if gd, ok := n.(*ast.GenDecl); ok && gd.Tok == token.CONST {
			return false
		}

		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}

		val, err := strconv.Unquote(lit.Value)
		if err != nil {
			val = lit.Value
		}

		// Filter out trivial/syntax strings
		if isTrivialString(val) {
			return true
		}

		counts[val] = append(counts[val], lit.Pos())
		return true
	})

	for val, positions := range counts {
		if len(positions) >= 2 {
			firstPos := fset.Position(positions[0])
			violations = append(violations, Violation{
				Pos:     firstPos,
				Kind:    "DUP",
				Message: fmt.Sprintf("string literal %q repeated %d times in file (extract to constant or config)", val, len(positions)),
			})
		}
	}

	return violations
}

func isTrivialString(s string) bool {
	if len(s) < 3 {
		return true
	}
	// Common format/whitespace/separator strings
	switch s {
	case "\n", "\r\n", "\t", " ", ", ", " - ", " -> ", ": ", "= ", "[]", "{}", "()", "...", "---":
		return true
	case "%s", "%v", "%d", "%w", "%q", "%t", "%x", "%064x", "%s\n", "%s: %v", "%s: %w", "%v\n":
		return true
	case "true", "false", "nil", "null", "none", "0", "1", "-1":
		return true
	case "json", "yaml", "text", "utf-8":
		return true
	case "POST", "GET", "PUT", "DELETE", "PATCH", "HEAD":
		return true
	}
	// Skip Go directives
	if strings.HasPrefix(s, "//go:") || strings.HasPrefix(s, "+build") {
		return true
	}
	return false
}

// ----------------------------------------------------------------------------
// Utilities
// ----------------------------------------------------------------------------

func detectPathsImport(repoRoot string) (string, error) {
	data, err := os.ReadFile(filepath.Join(repoRoot, "go.mod"))
	if err != nil {
		return "github.com/zqk-os/zqk/pkg/paths", nil
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			mod := strings.TrimSpace(strings.TrimPrefix(line, "module"))
			return mod + "/pkg/paths", nil
		}
	}
	return "github.com/zqk-os/zqk/pkg/paths", nil
}

func gitGoFiles(repo string) ([]string, error) {
	cmd := exec.Command("git", "-C", repo, "ls-files", "-z", "--", "*.go")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, p := range strings.Split(string(out), "\x00") {
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, "pkg/") || strings.HasPrefix(p, "cmd/") || strings.HasPrefix(p, "internal/") || strings.HasPrefix(p, "scripts/") {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

func heartbeat(ctx context.Context, total int, d time.Duration, cur *atomic.Int64, curPath *atomic.Value) {
	ticker := time.NewTicker(d)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			idx := cur.Load()
			if idx < 0 {
				fmt.Fprintf(os.Stderr, "check_path_and_perm_literals: starting (0/%d)...\n", total)
			} else {
				path := ""
				if v := curPath.Load(); v != nil {
					path, _ = v.(string)
				}
				fmt.Fprintf(os.Stderr, "check_path_and_perm_literals: progress scanned=%d/%d (%s)\n", idx+1, total, path)
			}
		}
	}
}
