package testing

import (
	"io/fs"
	stdtesting "testing"

	"github.com/lanceman/zqk/pkg/paths"
)

// MustMkdirAll calls [paths.EnsureDir] and fails the test on error. For several dirs under one root,
// prefer [paths.LayoutUnder] in the test (or in shared setup) and assert on [paths.Layout.Err].
func MustMkdirAll(t stdtesting.TB, path string, mode fs.FileMode) {
	t.Helper()
	if err := paths.EnsureDir(path, mode); err != nil {
		t.Fatal(err)
	}
}
