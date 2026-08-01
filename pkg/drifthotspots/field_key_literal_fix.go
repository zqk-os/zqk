// Package drifthotspots implements drift analysis and related source helpers.
//
// Go loop / iterator habit: when the standard library offers a single-pass iterator (iter.Seq), prefer
// `for v := range pkg.FooSeq(...)` over `for _, v := range pkg.Split(...)` (or manual append loops) if you
// only need to visit each element once. Examples: strings.SplitSeq / strings.Lines, bytes.SplitSeq,
// maps.Keys, maps.Values, slices.All / slices.Values. Same semantics as Split, but without allocating
// the intermediate slice.
//
// Field-key rewrites use the identifier "objects" for github.com/lanceman/zqk/pkg/objects. If a file
// already declares a parameter or local named objects (common for []map[string]any), rename that binding
// after a bulk fix so it does not shadow the package (e.g. testObjs, objSlice).
package drifthotspots

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/imports"
)

var fieldKeyConstLine = regexp.MustCompile(`^\s*(FieldKey[a-zA-Z0-9_]*)\s*=\s*"([^"]*)"\s*$`)

// LoadFieldKeyWireToConstName reads field_keys.go text and returns wire string -> Go const identifier (e.g. FieldKeyBatchID).
// Colliding wire values are an error (field_keys.go enforces uniqueness in practice).
func LoadFieldKeyWireToConstName(fieldKeysSrc []byte) (map[string]string, error) {
	out := make(map[string]string)
	for line := range strings.SplitSeq(string(fieldKeysSrc), "\n") {
		line = strings.TrimSpace(line)
		m := fieldKeyConstLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name, wire := m[1], m[2]
		if prev, ok := out[wire]; ok && prev != name {
			return nil, errfmt.Errorf("duplicate field key wire %q: %s and %s", wire, prev, name)
		}
		out[wire] = name
	}
	if len(out) == 0 {
		return nil, errfmt.Errorf("no FieldKey constants parsed from field_keys.go")
	}
	return out, nil
}

// LoadFieldKeyWireToConstFromModuleRoot reads field_keys.go under pkg/objects for an already-resolved module root.
func LoadFieldKeyWireToConstFromModuleRoot(moduleRoot string) (map[string]string, error) {
	fkPath, err := paths.ObjectsFieldKeysGoPath(moduleRoot)
	if err != nil {
		return nil, err
	}
	fkSrc, err := os.ReadFile(fkPath)
	if err != nil {
		return nil, errfmt.Errorf("read %s: %w", fkPath, err)
	}
	return LoadFieldKeyWireToConstName(fkSrc)
}

// LoadFieldKeyWireToConstFromModule finds the Go module root by walking up from startPath,
// reads field_keys.go under pkg/objects, and returns wire string -> Go const identifier.
func LoadFieldKeyWireToConstFromModule(startPath string) (map[string]string, error) {
	modRoot, err := paths.ModuleRootFromPath(startPath)
	if err != nil {
		return nil, errfmt.Newf("module root").Wrap(err)
	}
	return LoadFieldKeyWireToConstFromModuleRoot(modRoot)
}

// ShouldScanFieldKeyLiteralPath defines which repo-relative paths the field-key literal fixer scans;
// scripts/check-field-key-literals-repo.sh uses the same rules via go run ./scripts/fix_field_key_literals.
func ShouldScanFieldKeyLiteralPath(path string) bool {
	slash := filepath.ToSlash(path)
	if !strings.HasSuffix(path, ".go") {
		return false
	}
	if !strings.HasPrefix(slash, "cmd/") && !strings.HasPrefix(slash, "pkg/") && !strings.HasPrefix(slash, "internal/") {
		return false
	}
	switch {
	case slash == "pkg/objects/field_keys.go":
		return false
	case strings.HasPrefix(slash, "pkg/specbuilder/bldr_instance_v1/"):
		return false
	case strings.HasPrefix(slash, "pkg/specbuilder/bldr_v2/"):
		return false
	case strings.HasPrefix(slash, "pkg/specbuilder/builders/"):
		return false
	case strings.HasPrefix(slash, "pkg/specbuilder/trait_builders/"):
		return false
	}
	return true
}

type byteFix struct {
	start int
	end   int
	text  string
}

// FixFieldKeyLiteralsInFile returns updated source and the number of replacements, or (src, 0, nil) if none.
func FixFieldKeyLiteralsInFile(path string, src []byte, wireToConst map[string]string, pkgName string) ([]byte, int, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		return nil, 0, err
	}
	effectivePkg := pkgName
	if effectivePkg == "" && f.Name != nil {
		effectivePkg = f.Name.Name
	}

	var fixes []byteFix
	ast.Inspect(f, func(n ast.Node) bool {
		if n == nil {
			return true
		}
		bl, ok := n.(*ast.BasicLit)
		if !ok || bl.Kind != token.STRING {
			return true
		}
		wire, err := strconv.Unquote(bl.Value)
		if err != nil {
			return true
		}
		constName, known := wireToConst[wire]
		if !known {
			return true
		}
		pathNodes, ok := astutil.PathEnclosingInterval(f, bl.Pos(), bl.End())
		if !ok || len(pathNodes) < 2 {
			return true
		}
		if hasImportAncestor(pathNodes) {
			return true
		}
		if !isFieldKeyLiteralFixSite(pathNodes, bl) {
			return true
		}
		rep := replacementIdent(effectivePkg, constName)
		start := fset.Position(bl.Pos()).Offset
		end := fset.Position(bl.End()).Offset
		if start < 0 || end > len(src) || start >= end {
			return true
		}
		fixes = append(fixes, byteFix{start: start, end: end, text: rep})
		return true
	})
	if len(fixes) == 0 {
		return src, 0, nil
	}
	sort.Slice(fixes, func(i, j int) bool { return fixes[i].start > fixes[j].start })
	out := append([]byte(nil), src...)
	for _, fx := range fixes {
		out = append(out[:fx.start], append([]byte(fx.text), out[fx.end:]...)...)
	}
	out, err = imports.Process(path, out, nil)
	if err != nil {
		return nil, 0, errfmt.Newf("imports.Process").Wrap(err)
	}
	return out, len(fixes), nil
}

func replacementIdent(pkgName, constName string) string {
	if pkgName == "objects" {
		return constName
	}
	return "objects." + constName
}

func isFieldKeyLiteralFixSite(path []ast.Node, bl *ast.BasicLit) bool {
	if len(path) < 2 || path[0] != bl {
		return false
	}
	parent := path[1]
	switch p := parent.(type) {
	case *ast.KeyValueExpr:
		return p.Key == bl
	case *ast.IndexExpr:
		return p.Index == bl
	case *ast.CallExpr:
		if len(p.Args) == 0 || p.Args[0] != bl {
			return false
		}
		return isSetFieldFun(p.Fun)
	default:
		return false
	}
}

func isSetFieldFun(fun ast.Expr) bool {
	switch x := fun.(type) {
	case *ast.Ident:
		return x.Name == "SetField"
	case *ast.SelectorExpr:
		return x.Sel != nil && x.Sel.Name == "SetField"
	default:
		return false
	}
}

// WalkAndFixFieldKeyLiterals walks repoRoot, loads field keys from default path, and fixes each eligible file.
type WalkFieldKeyFixConfig struct {
	RepoRoot      string
	FieldKeysPath string // default pkg/objects/field_keys.go under RepoRoot
	Write         bool
	IncludeTests  bool
	PathPrefixes  []string // optional: only files whose repo-relative path has one of these prefixes
	FilePaths     []string // optional: exact repo-relative files (if set, ignores git ls-files walk)
}

// WalkFieldKeyFixResult summarizes a walk.
type WalkFieldKeyFixResult struct {
	FilesScanned int
	FilesChanged int
	TotalFixes   int
	ChangedPaths []string
	Errors       []string
}

// WalkAndFixFieldKeyLiterals lists Go files via git, applies FixFieldKeyLiteralsInFile, optionally writes.
func WalkAndFixFieldKeyLiterals(cfg WalkFieldKeyFixConfig) (*WalkFieldKeyFixResult, error) {
	res := &WalkFieldKeyFixResult{}
	if cfg.RepoRoot == "" {
		return nil, errfmt.Errorf("RepoRoot is empty")
	}
	fkPath := cfg.FieldKeysPath
	if fkPath == "" {
		var err error
		fkPath, err = paths.ObjectsFieldKeysGoPath(cfg.RepoRoot)
		if err != nil {
			return nil, errfmt.Newf("field_keys path").Wrap(err)
		}
	}
	fkSrc, err := os.ReadFile(fkPath)
	if err != nil {
		return nil, errfmt.Newf("read field_keys.go").Wrap(err)
	}
	wireToConst, err := LoadFieldKeyWireToConstName(fkSrc)
	if err != nil {
		return nil, err
	}

	var files []string
	if len(cfg.FilePaths) > 0 {
		for _, p := range cfg.FilePaths {
			p = filepath.ToSlash(strings.TrimSpace(p))
			if p == "" {
				continue
			}
			files = append(files, p)
		}
	} else {
		files, err = gitListGoFiles(cfg.RepoRoot)
		if err != nil {
			return nil, err
		}
	}

	for _, rel := range files {
		rel = filepath.ToSlash(rel)
		if !ShouldScanFieldKeyLiteralPath(rel) {
			continue
		}
		if !cfg.IncludeTests && strings.HasSuffix(rel, "_test.go") {
			continue
		}
		if len(cfg.PathPrefixes) > 0 {
			var match bool
			for _, pre := range cfg.PathPrefixes {
				pre = filepath.ToSlash(strings.TrimSuffix(strings.TrimSpace(pre), "/"))
				if pre == "" {
					continue
				}
				if rel == pre || strings.HasPrefix(rel, pre+"/") {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		abs := filepath.Join(cfg.RepoRoot, rel)
		src, err := os.ReadFile(abs)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: read: %v", rel, err))
			continue
		}
		if isGeneratedSource(src) {
			continue
		}
		res.FilesScanned++
		newSrc, n, err := FixFieldKeyLiteralsInFile(abs, src, wireToConst, "")
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", rel, err))
			continue
		}
		if n == 0 {
			continue
		}
		res.TotalFixes += n
		if cfg.Write {
			mode := os.FileMode(0o600)
			if fi, statErr := os.Stat(abs); statErr == nil {
				mode = fi.Mode() & 0o777
			}
			if err := os.WriteFile(abs, newSrc, mode); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("%s: write: %v", rel, err))
				continue
			}
		}
		res.FilesChanged++
		res.ChangedPaths = append(res.ChangedPaths, rel)
	}
	return res, nil
}

func gitListGoFiles(repoRoot string) ([]string, error) {
	cmd := exec.Command("git", "-C", repoRoot, "ls-files", "-z", "--", "*.go")
	out, err := cmd.Output()
	if err != nil {
		return nil, errfmt.Newf("git ls-files").Wrap(err)
	}
	if len(out) == 0 {
		return nil, nil
	}
	parts := bytes.Split(out, []byte{0})
	var paths []string
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		paths = append(paths, string(p))
	}
	return paths, nil
}
