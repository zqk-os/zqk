package search

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// resolveTrigramCachePath returns the persistent trigram index cache path for projectRoot.
// When projectRoot already has an initialized `.zqk/` workspace directory, the cache is stored
// inside `<projectRoot>/.zqk/cache/trigram.idx`. Otherwise (standalone zero-pollution mode),
// the cache is stored outside the working tree under `$XDG_CACHE_HOME/<brand>/<repo-hash>/trigram.idx`
// (falling back to `os.UserCacheDir()`) so `git status` remains 100% clean.
func resolveTrigramCachePath(projectRoot string) string {
	projectDataDir := filepath.Join(projectRoot, paths.ProjectDataDir)
	if fileutil.Exists(projectDataDir) {
		return filepath.Join(projectDataDir, paths.CacheDir, "trigram.idx")
	}

	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		absRoot = filepath.Clean(projectRoot)
	}
	sum := sha256.Sum256([]byte(absRoot))
	repoKey := hex.EncodeToString(sum[:8])

	baseDir := strings.TrimSpace(os.Getenv("XDG_CACHE_HOME"))
	if baseDir == "" {
		if userCache, cacheErr := os.UserCacheDir(); cacheErr == nil && strings.TrimSpace(userCache) != "" {
			baseDir = userCache
		} else {
			baseDir = os.TempDir()
		}
	}
	return filepath.Join(baseDir, brand.ExecutableName(), repoKey, "trigram.idx")
}
