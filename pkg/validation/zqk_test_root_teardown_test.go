package validation

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/projecttemp"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// runZQKTempTestRootTeardown runs the shared isolated-root strip pipeline. pkg/validation cannot import
// pkg/storage or pkg/testkit (import cycles); [projecttemp.RunIsolatedRootStrip] uses the same pipeline-backed
// strip as storage's STRIP_PROCESS_ARTIFACTS stage.
func runZQKTempTestRootTeardown(t *testing.T, root string) {
	t.Helper()
	if err := projecttemp.RunIsolatedRootStrip(root); err != nil {
		t.Logf("isolated root strip pipeline: %v", err)
	}
}

// registerZQKTestRootForTest sets ZQK_TEST_ROOT on a fresh [testing.T.TempDir], runs [setupTestEnvironment],
// and registers [projecttemp.RunIsolatedRootStrip] plus Unsetenv on cleanup. Use instead of ad hoc
// Setenv/defer Unsetenv so temp-root teardown stays aligned with storage/testkit.
func registerZQKTestRootForTest(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	_ = zqkenv.TestRoot().Set(tmp)
	root, err := setupTestEnvironment(tmp)
	if err != nil {
		_ = zqkenv.TestRoot().Unset()
		t.Fatalf("register ZQK test root: setup test environment: %v", err)
	}
	t.Cleanup(func() {
		runZQKTempTestRootTeardown(t, root)
		_ = zqkenv.TestRoot().Unset()
	})
	return root
}
