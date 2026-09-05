package filecas

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

// cleanupStaleCASIndexTempFiles removes abandoned CreateTemp siblings left when a
// process is killed between CreateTemp and Rename (IDE runner timeouts are a
// common cause). Never removes the just-renamed tmpName.
//
// TRACK: REDACTED
func cleanupStaleCASIndexTempFiles(dir, indexBase, justWroteTmp string) {
	if dir == emptyValue || indexBase == emptyValue {
		return
	}
	prefix := indexBase + ".tmp-"
	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-2 * time.Minute)
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		full := filepath.Join(dir, name)
		if full == justWroteTmp {
			continue
		}
		info, iErr := e.Info()
		if iErr != nil {
			continue
		}
		if info.ModTime().After(cutoff) {
			continue // likely an in-flight concurrent writer
		}
		_ = fileutil.RemoveFile(full) //nolint:errcheck // best-effort sweep
	}
}
