package paths

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

const goModFileName = "go.mod"

// ModuleRootFromPath walks upward from startPath until it finds a directory containing go.mod,
// and returns that directory as a clean absolute path. startPath may be a file or directory.
func ModuleRootFromPath(startPath string) (string, error) {
	dir := startPath
	info, err := os.Stat(startPath)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		dir = filepath.Dir(startPath)
	}
	absStart, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for d := absStart; ; d = filepath.Dir(d) {
		candidate := filepath.Join(d, goModFileName)
		if _, err := os.Stat(candidate); err == nil {
			return filepath.Clean(d), nil
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", errfmt.Errorf("%s not found ascending from %s", goModFileName, absStart)
		}
	}
}

// ObjectsFieldKeysGoPath returns the absolute path to field_keys.go under the given module root
// (directory that contains go.mod).
func ObjectsFieldKeysGoPath(moduleRoot string) (string, error) {
	if moduleRoot == emptyValue {
		return "", errfmt.Errorf("module root is empty")
	}
	return filepath.Abs(filepath.Join(moduleRoot, PkgDir, ObjectsPackageDir, FieldKeysGoFile))
}

// ModuleImportPath reads the `module` directive from go.mod under moduleRoot and returns the module path
// (e.g. github.com/org/repo). Used by codegen to emit import paths without hardcoding the module prefix.
func ModuleImportPath(moduleRoot string) (string, error) {
	if moduleRoot == emptyValue {
		return "", errfmt.Errorf("module root is empty")
	}
	data, err := os.ReadFile(filepath.Join(moduleRoot, goModFileName)) //nolint:gosec
	if err != nil {
		return "", errfmt.Newf("read go.mod").Wrap(err)
	}
	return parseModuleImportPath(data)
}

func parseModuleImportPath(data []byte) (string, error) {
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		if !strings.HasPrefix(line, "module ") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(line, "module"))
		if rest == "" || rest[0] == '(' {
			continue
		}
		if i := strings.Index(rest, "//"); i >= 0 {
			rest = strings.TrimSpace(rest[:i])
		}
		rest = strings.Trim(rest, `"`)
		if rest == "" {
			return "", errfmt.Errorf("empty module path in go.mod")
		}
		return rest, nil
	}
	return "", errfmt.Errorf("module directive not found in go.mod")
}

// ConfigImportPath returns the Go import path for pkg/config under moduleImport.
func ConfigImportPath(moduleImport string) string {
	return path.Join(moduleImport, PkgDir, ConfigPackageDir)
}

// ObjectsImportPath returns the Go import path for pkg/objects under moduleImport.
func ObjectsImportPath(moduleImport string) string {
	return path.Join(moduleImport, PkgDir, ObjectsPackageDir)
}

// SpecbuilderPackageImportPath returns the Go import path for pkg/specbuilder/<childDirName>
// (childDirName is typically the basename of the codegen output directory, e.g. routing_builders).
func SpecbuilderPackageImportPath(moduleImport, childDirName string) string {
	return path.Join(moduleImport, PkgDir, SpecbuilderDir, childDirName)
}
