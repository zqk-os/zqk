package storage

import (
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestDKM(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot(), tmpDir)
	bootstrapTestRootFromProjectRoot(t, tmpDir, moduleRootFromGoEnv(t))

	mapper := objects.GetGlobalKindMapper()
	_ = mapper.Initialize()

	dir := mapper.GetDirectoryFromKind("kind_synonym")
	t.Logf("While ZQK_TEST_ROOT is set: kind_synonym -> %q", dir)

	// Clean up
	opts := TempProjectTeardown(tmpDir, nil)
	RunProjectTestTeardown(opts)

	t.Setenv(zqkenv.TestRoot(), "")

	dir2 := mapper.GetDirectoryFromKind("kind_synonym")
	t.Logf("After ZQK_TEST_ROOT is cleared and scrubbed: kind_synonym -> %q", dir2)
}
