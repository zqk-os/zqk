//go:build !production
// +build !production

package storage

import (
	"testing"

	"github.com/lanceman/zqk/pkg/testenvroot"
)

// MustEnsureProcessSpecsLayoutForTest creates .zqk/process and object_specs under root for tests that
// previously duplicated os.MkdirAll blocks. Prefer this or [paths.EnsureProcessAndObjectSpecsLayout]
// over string literals and raw octal modes.
// Also copies object_specs and lifecycles from the module checkout so FileObjectStorage Create
// can load kind lifecycles (empty TEST_ROOT dirs used to fail with "failed to read lifecycle file").
func MustEnsureProcessSpecsLayoutForTest(t *testing.T, root string) {
	t.Helper()
	modRoot := moduleRootFromGoEnv(t)
	if err := testenvroot.BootstrapRoot(root, modRoot); err != nil {
		t.Fatalf("bootstrap root: %v", err)
	}
}

func mustEnsureProcessSpecsLayout(t *testing.T, dest string) {
	MustEnsureProcessSpecsLayoutForTest(t, dest)
}

func ResetReverseReferenceIndexForTest(t *testing.T) {
	t.Helper()
	if old := revRefPersistTimer.Swap(nil); old != nil {
		_ = old.Stop()
	}
	revRefBoundRoot.Store(emptyValue)
}
