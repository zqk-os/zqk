package storage

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// allowedStorageProcessSetters is the GLOBALS-DI inventory (CRIT-CEF-R2-ARCH-GLOBALS-DI-A).
// Composition root: CLI OnStorageCreated installs cache + lifecycle + change-notify handlers.
// Do not add package-level SetGlobal* or extra Set*Handler funcs.
var allowedStorageProcessSetters = []string{
	"SetCacheChecker",
	"SetCacheOperationHandler",
	"SetChangeNotificationHandler",
	"SetGlobalCacheProvider",
	"SetLifecycleHookHandler",
}

func TestStorageProcessSettersStayOnAllowlist(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	root := filepath.Dir(thisFile)
	fset := token.NewFileSet()
	var found []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && (d.Name() == "vendor" || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		for _, decl := range f.Decls {
			fn, isFn := decl.(*ast.FuncDecl)
			if !isFn || fn.Recv != nil || fn.Name == nil {
				continue
			}
			name := fn.Name.Name
			if strings.HasPrefix(name, "SetGlobal") || isProcessWideHandlerSetter(name) {
				found = append(found, name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(found)
	found = slices.Compact(found)
	allow := append([]string(nil), allowedStorageProcessSetters...)
	slices.Sort(allow)
	if !slices.Equal(found, allow) {
		t.Errorf("pkg/storage process-wide setters drifted from GLOBALS-DI allowlist\n  found: %v\n  allow: %v\n  add constructors, do not add SetGlobal*/Set*Handler package funcs", found, allow)
	}
}

func isProcessWideHandlerSetter(name string) bool {
	switch name {
	case "SetCacheOperationHandler", "SetCacheChecker", "SetLifecycleHookHandler", "SetChangeNotificationHandler":
		return true
	default:
		return false
	}
}

// TestStorageProcessSetters_BLI_CEF_R17_GLOBALS_DI_001 verifies that package-level setter injection
// in pkg/storage is frozen and restricted to explicit dependencies (BLI-CEF-R17-GLOBALS-DI-001).
func TestStorageProcessSetters_BLI_CEF_R17_GLOBALS_DI_001(t *testing.T) {
	TestStorageProcessSettersStayOnAllowlist(t)
}
