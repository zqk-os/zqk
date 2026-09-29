// Extracted from object_storage_file.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"github.com/zqk-os/zqk/pkg/when"

	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage/filecas"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func (f *FileObjectStorage) getObjectFilePath(id, kind string) (string, error) {
	// Stream-backed: prefer stream_current overlay (runtime delta), else stream registry
	if StreamStorageEnabledForKind(kind) {
		if overlay := GetStreamBackedObjectFilePath(f.projectRoot, kind, id); overlay != emptyValue {
			return overlay, nil
		}
		if loc := f.getStreamLocation(id, kind); loc != emptyValue {
			return loc, nil
		}
		return "", errfmt.Errorf(ConstStreamObjectStrKindStrNotFoundInStreamErr, id, kind, ErrObjectNotFound)
	}

	if p := cachedLivePath(id); p != emptyValue {
		return p, nil
	}

	if globalCacheProvider != nil {
		if k, path, found := globalCacheProvider.LocateObject(id); found && (kind == emptyValue || k == kind) {
			if f.projectRoot == emptyValue || strings.HasPrefix(path, f.projectRoot) {
				if _, err := fileutil.Stat(path); err == nil {
					return path, nil
				}
			}
		}
	}

	kindDir := f.GetKindDir(kind)
	if kindDir == "" {
		return "", errfmt.Errorf(ConstStreamUnknownObjectKindStr, kind)
	}

	// Check if this kind uses content-addressable storage
	// Resolve via CAS index first. Do NOT full-scan the kind dir on every index miss —
	// that is O(files) and hang-prone (see core-backlog). Bulk
	// repair paths use discoverCASFilePathsByScanning once for many IDs.
	if f.usesContentAddressableStorage(kind) {
		var cas *filecas.ContentAddressableStorage
		if c, err := f.getContentAddressableStorage(kind); err == nil {
			cas = c
			if hashPath, err := cas.GetFilePathForID(id); err == nil {
				return hashPath, nil
			}
		}

		config := GetStorageConfig()
		accountDir := objects.GetDirectoryFromKind(objects.KindAccount)
		var legacyFilename string
		when.When(func() bool {
			return accountDir == "accounts" && objects.GetDirectoryFromKind(kind) == accountDir && strings.HasPrefix(id, "account:")
		}).Then(func() {
			username := strings.TrimPrefix(id, "account:")
			legacyFilename = fmt.Sprintf("account-%s%s", username, config.YAMLExtension)
		}).OrElse(func() {
			legacyFilename = fmt.Sprintf("%s%s", id, config.YAMLExtension)
		}).Run()
		legacyPath := filepath.Join(kindDir, legacyFilename)
		if _, statErr := fileutil.Stat(legacyPath); statErr == nil {
			return legacyPath, nil
		}

		// Cold/broken CAS only: one discovery scan to rehydrate a missing index entry.
		// When CAS is available, index miss ⇒ draft-plane heal, else not found (fail fast).
		if cas == nil {
			discoveredPath, _, scanErr := filecas.DiscoverCASFilePathByScanning(id, kindDir)
			if scanErr == nil {
				return discoveredPath, nil
			}
			if f.objectDraftPlaneExists(kind, id) {
				return f.objectDraftPlanePath(kind, id), nil
			}
			return "", errfmt.Errorf(ConstStreamObjectStrKindStrNotFoundValErr, id, kind, scanErr, ErrObjectNotFound)
		}
		if f.objectDraftPlaneExists(kind, id) {
			draftPath := f.objectDraftPlanePath(kind, id)
			_ = coupleObjectIDCacheLivePath(id, kind, draftPath)
			return draftPath, nil
		}
		return "", errfmt.Errorf(ConstStreamObjectStrKindStrNotFoundValErr, id, kind, "cas index miss", ErrObjectNotFound)
	}

	// Legacy fallback: only reached when usesContentAddressableStorage(kind) is false.
	// All kinds are CAS in practice; this path exists only for migration/backward compatibility.
	// For accounts, use account-{username}.yaml; for others, use {id}.yaml.
	config := GetStorageConfig()
	// Check if this is an account kind by checking if directory is "accounts"
	accountDir := objects.GetDirectoryFromKind(objects.KindAccount)
	var filename string
	when.When(func() bool {
		return accountDir == "accounts" && objects.GetDirectoryFromKind(kind) == accountDir && strings.HasPrefix(id, "account:")
	}).Then(func() {
		username := strings.TrimPrefix(id, "account:")
		filename = fmt.Sprintf("account-%s%s", username, config.YAMLExtension)
	}).OrElse(func() {
		filename = fmt.Sprintf("%s%s", id, config.YAMLExtension)
	}).Run()

	// Check if this kind uses bucketed storage (e.g., audit_event in audit/YYYY-MM/)
	if f.usesBucketedStorage(kind, kindDir) {
		// For bucketed storage, optimize by checking most likely months first
		// Strategy: Check current month, last month, and a few recent months
		// This avoids expensive full directory scans for the common case
		now := time.Now()
		monthsToCheck := []string{
			now.Format("2006-01"),                   // Current month (most likely)
			now.AddDate(0, -1, 0).Format("2006-01"), // Last month
			now.AddDate(0, -2, 0).Format("2006-01"), // 2 months ago
		}

		// Fast path: Try likely months first (just stat, no directory listing)
		// This is O(1) per file and very fast
		for _, month := range monthsToCheck {
			dateDir := filepath.Join(kindDir, month)
			filePath := filepath.Join(dateDir, filename)
			if _, err := fileutil.Stat(filePath); err == nil {
				return filePath, nil
			}
		}

		// Slow path: If not found in likely months, we need to search
		// Cache bucket subdir names per kindDir so multiple getObjectFilePath calls don't each ReadDir
		var bucketNames []string
		if val, ok := f.bucketListCache.Load(kindDir); ok {
			bucketNames = val.([]string)
		} else {
			entries, readErr := fileutil.ReadDir(kindDir)
			if readErr != nil {
				if !fileutil.IsNotExist(readErr) {
					return "", errfmt.Newf(ConstStreamFailedToReadDirectory).Wrap(readErr)
				}
				bucketNames = []string{} // kindDir missing; don't cache, fall through
			} else {
				for _, entry := range entries {
					if entry.IsDir() {
						bucketNames = append(bucketNames, entry.Name())
					}
				}
				f.bucketListCache.Store(kindDir, bucketNames)
			}
		}

		// Build set of already-checked months for efficiency
		checkedSet := make(map[string]bool)
		for _, m := range monthsToCheck {
			checkedSet[m] = true
		}

		// Search through remaining subdirectories
		// Only do this if fast path failed (should be rare)
		// Search ALL subdirectories - works with any bucket strategy (not just date-based)
		for _, bucketName := range bucketNames {
			// Skip already-checked directories (from fast path)
			if checkedSet[bucketName] {
				continue
			}
			// Check all subdirectories (not just date patterns) - supports any bucket strategy
			bucketDir := filepath.Join(kindDir, bucketName)
			filePath := filepath.Join(bucketDir, filename)
			if _, err := fileutil.Stat(filePath); err == nil {
				return filePath, nil
			}
		}
		// If not found in any date directory, return the path without date (for backward compatibility)
		// This will cause a "not found" error, which is correct behavior
	}

	return filepath.Join(kindDir, filename), nil
}

// ============================================================================
// Extracted Operations
// ============================================================================
//
// The following operations have been extracted to separate files:
// - CRUD operations (Create, Read, Update, Delete, Move) -> object_storage_file_crud.go
// - Bulk operations (BulkCreate, BulkUpdate, BulkGet, BulkDelete) -> object_storage_file_bulk.go
// - Transaction operations (BeginTransaction, BeginEnhancedTransaction) -> object_storage_file_transaction.go
// - Graph operations (GetRelated, GetPath, GetNeighbors) -> object_storage_file_graph.go
// - List/Query operations (List, Query, Count, Exists) -> object_storage_file_list.go
// - Search operations -> object_storage_file_search.go
// - Validation operations -> object_storage_file_validation.go
// - Helper functions (file I/O, hash, metadata, permissions) -> object_storage_file_helpers.go
//
// This file now contains only core infrastructure:
// - Global variables and handlers
// - FileObjectStorage struct definition
// - NewFileObjectStorage() constructor
// - Core getters and infrastructure methods
// - CAS detection and management
// - Hash registry creation
// - File path resolution

type contextKeySkipWriteBehind struct{}
