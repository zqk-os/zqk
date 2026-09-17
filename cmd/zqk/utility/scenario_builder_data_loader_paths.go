package utility

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/datacell"

	"github.com/lanceman/zqk/cmd/zqk/system"

	"github.com/lanceman/zqk/pkg/objects"
)

// getObjectFilePath gets the file path for an object from storage
// This is used to update the ObjectIDCache immediately after creation
func (sb *ScenarioBuilder) getObjectFilePath(id, kind string) string {
	// Try to get file path from ObjectIDCache if it's already there
	// The cache should be updated by the CacheOperationHandler callback during Create()
	cache := system.GetGlobalObjectIDCache()
	entry, exists := cache.Get(id)
	if exists && entry != nil && entry.FilePath != emptyValue {
		return entry.FilePath
	}

	// Cache entry not found - reconstruct path
	return sb.reconstructFilePath(id, kind)
}

// updateObjectIDCacheSync updates the ObjectIDCache synchronously
// This ensures the cache is updated immediately after object creation for reference validation
func (sb *ScenarioBuilder) updateObjectIDCacheSync(id, kind, filePath string) error {
	return system.UpdateObjectIDCache(id, kind, filePath)
}

// reconstructFilePath reconstructs the file path for an object based on its ID and kind
// This is used when the cache doesn't have the path yet (e.g., during batch creation)
// Note: This may not be accurate for bucketed storage, but it's better than nothing
func (sb *ScenarioBuilder) reconstructFilePath(id, kind string) string {
	if sb.config == nil || sb.config.TargetDir == emptyValue {
		return ""
	}

	// Get directory name for this kind
	dirName := objects.GetDirectoryFromKind(kind)
	if dirName == emptyValue {
		return ""
	}

	// Reconstruct path based on kind and ID
	// For most objects: .zqk/process/{dirName}/{id}.yaml
	// For CAS objects: .zqk/process/{dirName}/{hash}.yaml (but we can't compute hash here)
	// For accounts: .zqk/process/{dirName}/account-{username}.yaml
	if kind == objects.KindAccount {
		// Accounts use account-{username}.yaml format
		if strings.HasPrefix(id, "account:") {
			username := strings.TrimPrefix(id, "account:")
			filename := fmt.Sprintf("account-%s.yaml", username)
			return filepath.Join(datacell.CellCASPrimaryDir(sb.config.TargetDir, dirName), filename)
		}
		// If ID is not in account:username format, use standard format
		filename := fmt.Sprintf("%s.yaml", id)
		return filepath.Join(datacell.CellCASPrimaryDir(sb.config.TargetDir, dirName), filename)
	}

	// For non-CAS objects, use standard format: {id}.yaml
	// For CAS objects, this won't be accurate (they use hash-based filenames)
	// but it's better than nothing for cache purposes
	filename := fmt.Sprintf("%s.yaml", id)
	return filepath.Join(datacell.CellCASPrimaryDir(sb.config.TargetDir, dirName), filename)
}
