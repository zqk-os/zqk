package validation

import (
	"os"
	"testing"

	"github.com/lanceman/zqk/pkg/projecttemp"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// runZQKTempTestRootTeardown runs the shared isolated-root strip pipeline. pkg/validation cannot import
// pkg/storage or pkg/testkit (import cycles); [projecttemp.RunIsolatedRootStrip] uses the same pipeline-backed
// strip as storage's STRIP_PROCESS_ARTIFACTS stage.
func runZQKTempTestRootTeardown(t *testing.T, root string) {
	t.Helper()
	if err := projecttemp.RunIsolatedRootStrip(root); err != nil {
		t.Logf(ConstMagic034593ae, err)
	}
}

// registerZQKTestRootForTest sets ZQK_TEST_ROOT on a fresh [testing.T.TempDir], runs [setupTestEnvironment],
// and registers [projecttemp.RunIsolatedRootStrip] plus Unsetenv on cleanup. Use instead of ad hoc
// Setenv/defer Unsetenv so temp-root teardown stays aligned with storage/testkit.
func registerZQKTestRootForTest(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	_ = os.Setenv(zqkenv.TestRoot(), tmp)
	root, err := setupTestEnvironment(tmp)
	if err != nil {
		_ = os.Unsetenv(zqkenv.TestRoot())
		t.Fatalf(ConstMagicef54f9d1, err)
	}
	t.Cleanup(func() {
		runZQKTempTestRootTeardown(t, root)
		_ = os.Unsetenv(zqkenv.TestRoot())
	})
	return root
}
