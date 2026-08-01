package storage

import (
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
)

// mustEnsureProcessSpecsLayout creates docs/process and object_specs under root for tests that
// previously duplicated os.MkdirAll blocks. Prefer this or [paths.EnsureProcessAndObjectSpecsLayout]
// over string literals and raw octal modes.
func mustEnsureProcessSpecsLayout(t *testing.T, root string) {
	t.Helper()
	if err := paths.EnsureProcessAndObjectSpecsLayout(root); err != nil {
		t.Fatalf("ensure process/specs layout: %v", err)
	}
}
