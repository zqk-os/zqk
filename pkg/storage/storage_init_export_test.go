package storage

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/testenvroot"
)

// MustBenchmarkSetupTestRootWithLayoutAndSpecs mirrors pkg/testing SetupTestEnvironment plus
// CopySpecsToTestRoot for benchmarks (layout, test-settings, copied object_specs YAML).
func MustBenchmarkSetupTestRootWithLayoutAndSpecs(b *testing.B, root string) {
	b.Helper()
	absRoot, err := filepath.Abs(root)
	if err != nil {
		b.Fatal(err)
	}
	modRoot := moduleRootFromGoEnvTB(b)
	if err := testenvroot.BootstrapRoot(absRoot, modRoot); err != nil {
		b.Fatal(err)
	}
}
