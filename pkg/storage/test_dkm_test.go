package storage

import (
	"os"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestDKM(t *testing.T) {
	orig := os.Getenv(zqkenv.TestRoot())
	defer func() {
		if orig != "" {
			_ = os.Setenv(zqkenv.TestRoot(), orig)
		} else {
			_ = os.Unsetenv(zqkenv.TestRoot())
		}
	}()

	tmpDir := t.TempDir()
	os.Setenv(zqkenv.TestRoot(), tmpDir)
	bootstrapTestRootFromProjectRoot(t, tmpDir, moduleRootFromGoEnv(t))

	mapper := objects.GetGlobalKindMapper()
	_ = mapper.Initialize()

	dir := mapper.GetDirectoryFromKind("kind_synonym")
	t.Logf("While ZQK_TEST_ROOT is set: kind_synonym -> %q", dir)

	// Clean up
	opts := TempProjectTeardown(tmpDir, nil)
	RunProjectTestTeardown(opts)

	os.Setenv(zqkenv.TestRoot(), "")

	dir2 := mapper.GetDirectoryFromKind("kind_synonym")
	t.Logf("After ZQK_TEST_ROOT is cleared and scrubbed: kind_synonym -> %q", dir2)
}
