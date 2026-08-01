package storage

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
)

// MustBenchmarkSetupTestRootWithLayoutAndSpecs mirrors pkg/testing SetupTestEnvironment plus
// CopySpecsToTestRoot for benchmarks (layout, test-settings, copied object_specs YAML).
func MustBenchmarkSetupTestRootWithLayoutAndSpecs(b *testing.B, root string) {
	b.Helper()
	absRoot, err := filepath.Abs(root)
	if err != nil {
		b.Fatal(err)
	}
	if err := paths.LayoutUnder(absRoot).
		Dir(paths.ProjectDataDir, paths.DirPerm755).
		Dir(paths.ProcessDir, paths.DirPerm755).
		Dir(paths.ProcessInternalObjectSpecsDir, paths.DirPerm755).
		Dir(paths.ProcessInternalTraitsDir, paths.DirPerm755).
		Err(); err != nil {
		b.Fatal(err)
	}
	if err := writeMinimalTestSettingsYAMLForTestRoot(absRoot); err != nil {
		b.Fatal(err)
	}
	modRoot := moduleRootFromGoEnvTB(b)
	if err := copyObjectSpecYAMLFilesToTestRoot(absRoot, modRoot); err != nil {
		b.Fatal(err)
	}
}
