package objectidcache

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPkgObjectIDCacheMustNotImportCLI(t *testing.T) {
	t.Parallel()
	banned := []string{
		"github.com/zqk-os/zqk/pkg/cliapp",
		"github.com/zqk-os/zqk/pkg/cli",
		"github.com/zqk-os/zqk/cmd/zqk",
	}
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	root := filepath.Dir(thisFile)
	fset := token.NewFileSet()
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
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		f, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			for _, b := range banned {
				if p == b || strings.HasPrefix(p, b+"/") {
					t.Errorf("%s imports banned package %s", path, p)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestObjectIDCache_BLI_CEF_R17_SYSTEM_TRANCHE2_001 verifies that ObjectIDCache is extracted
// into pkg/objectidcache and does not import CLI packages (BLI-CEF-R17-SYSTEM-TRANCHE2-001).
func TestObjectIDCache_BLI_CEF_R17_SYSTEM_TRANCHE2_001(t *testing.T) {
	TestPkgObjectIDCacheMustNotImportCLI(t)
}
