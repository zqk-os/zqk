package brand

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestRootGoModReplacesAreNestedModulesOrAbsent encodes CRIT-CEF-R2-ARCH-GOMOD-REPLACES-A:
// replace count 0, or each replace target is a real nested module (has its own go.mod).
func TestRootGoModReplacesAreNestedModulesOrAbsent(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	root, err := findGoModRoot(filepath.Dir(thisFile))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "go.mod")) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}
	var fiction []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "replace ") {
			continue
		}
		target := replaceLocalTarget(line)
		if target == "" {
			fiction = append(fiction, line)
			continue
		}
		nested := filepath.Join(root, filepath.Clean(target), "go.mod")
		if _, statErr := os.Stat(nested); statErr != nil {
			fiction = append(fiction, line)
		}
	}
	if len(fiction) > 0 {
		t.Fatalf("go.mod replace directives must be absent or point at a nested module with go.mod (BLI-CEF-R2-ARCH-GOMOD-REPLACES); fiction:\n  %s", strings.Join(fiction, "\n  "))
	}
}

func findGoModRoot(start string) (string, error) {
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

func replaceLocalTarget(line string) string {
	// replace old => ./pkg/foo   or  replace old => ./pkg/foo v0.0.0
	idx := strings.Index(line, "=>")
	if idx < 0 {
		return ""
	}
	rhs := strings.TrimSpace(line[idx+2:])
	fields := strings.Fields(rhs)
	if len(fields) == 0 {
		return ""
	}
	path := fields[0]
	if strings.HasPrefix(path, "./") || strings.HasPrefix(path, "../") {
		return path
	}
	return ""
}
