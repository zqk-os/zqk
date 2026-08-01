package projecttemp

import (
	"os"
	"path/filepath"

	"github.com/lanceman/zqk/pkg/paths"
)

const emptyValue = ""

// IsProbableGitWorktreeRoot reports whether dir contains Git metadata at [paths.GitWorktreeMetadataEntry]
// (file or directory). Destructive temp teardown must not run when this is true.
func IsProbableGitWorktreeRoot(dir string) bool {
	if dir == emptyValue {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, paths.GitWorktreeMetadataEntry))
	return err == nil
}
