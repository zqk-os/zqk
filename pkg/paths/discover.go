package paths

import (
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/stampmemo"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// discoveredFromCwd is keyed by cwd+\0+relativePath (cwd × closed relative names).
// Stamp is the cwd. ResetCwdDiscovery after tests that chdir.
var discoveredFromCwd stampmemo.Table[string]

// ResetCwdDiscovery forgets cwd walks because the key space (working directory) changed.
// Domain-file memos are reset too: tests that chdir then materialize specs must not
// keep a miss from the previous working directory.
func ResetCwdDiscovery() {
	discoveredFromCwd.Reset()
	domainFiles.Reset()
}

// FirstExistingFromCwd walks from the working directory toward the filesystem root
// and returns the first existing path for rel. Missing is "".
func FirstExistingFromCwd(rel string) string {
	if rel == "" {
		return ""
	}
	wd, err := fileutil.Getwd()
	if err != nil {
		return ""
	}
	hit, _ := discoveredFromCwd.Load(wd+"\x00"+rel, stampmemo.Of(wd), func() (string, error) {
		dir := wd
		for {
			candidate := filepath.Join(dir, rel)
			if stampmemo.Of(candidate) != 0 {
				return candidate, nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				return "", nil
			}
			dir = parent
		}
	})
	return hit
}

// FirstExistingFromCwdAny returns the first existing candidate, trying each
// relative path in order (cwd toward filesystem root per name).
func FirstExistingFromCwdAny(rels []string) string {
	for _, rel := range rels {
		if hit := FirstExistingFromCwd(rel); hit != "" {
			return hit
		}
	}
	return ""
}
