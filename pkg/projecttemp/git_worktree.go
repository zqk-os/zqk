package projecttemp

import (
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const emptyValue = ""

// IsProbableGitWorktreeRoot reports whether dir contains Git metadata at [paths.GitWorktreeMetadataEntry]
// (file or directory). Destructive temp teardown must not run when this is true.
func IsProbableGitWorktreeRoot(dir string) bool {
	if dir == emptyValue {
		return false
	}
	_, err := fileutil.Stat(filepath.Join(dir, paths.GitWorktreeMetadataEntry))
	return err == nil
}
