package system

import (
	"github.com/lanceman/zqk/pkg/datacell"

	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"gopkg.in/yaml.v3"
)

// casFixWaitTimeout is the max time to wait for a CAS index update callback (avoids hanging tests/CLI).
const casFixWaitTimeout = 30 * time.Second

const (
	casErrAlreadyExists          = "already exists"
	casMsgRemoveStaleIndexFailed = "Failed to remove stale index entry (non-critical)"
	casMsgFlushIndexWarn         = "Failed to flush CAS index (non-critical, index will be saved eventually)"
)

// resolveCASFixFilePathAndContent resolves the file path and reads content for CAS auto-fix.
// If the discovery path fails to read (e.g. wrong-kind), it looks up by object ID and retries.
// Returns resolved filePath, kind, content, and error.
func resolveCASFixFilePathAndContent(projectRoot string, fixCtx *AutoFixContext, fixKind string) (filePath string, kind string, content []byte, err error) {
	filePath = fixCtx.FilePath
	kind = fixKind
	content, err = os.ReadFile(filePath)
	if err != nil {
		if foundPath, foundKind := findObjectByID(projectRoot, fixCtx.Obj.ID); foundPath != emptyValue {
			filePath = foundPath
			fixCtx.FilePath = foundPath
			if foundKind != emptyValue {
				kind = foundKind
			}
			content, err = os.ReadFile(filePath)
		}
	}
	return filePath, kind, content, err
}

// fixCASIndexOutOfSync repairs CAS index drift by forcing the index mapping to match the content hash
// of the file on disk. If the file is not yet hash-addressed, this will also migrate it to CAS by
// writing the hash-addressed file and updating the mapping.
// storageProvider should be the same instance used for the check so fixes persist correctly.
func fixCASIndexOutOfSync(fixCtx *AutoFixContext, fixKind string, storageProvider storage.ObjectStorageProvider) (bool, string) {
	projectRoot := fixCtx.Ctx.ProjectRoot
	projectRoot = ProjectRootOrResolve(projectRoot)
	if projectRoot == emptyValue {
		logging.Fluent(fixCtx.Logger).Warn("Cannot fix CAS index drift: no project root").
			String("object_id", fixCtx.Obj.ID).
			Log()
		return false, ""
	}

	// Read file content (source of truth). If discovery path is wrong-kind, resolve by ID and use that kind.
	filePath, fixKind, fileContent, err := resolveCASFixFilePathAndContent(projectRoot, fixCtx, fixKind)
	if err != nil {
		logging.Fluent(fixCtx.Logger).Warn("Cannot fix CAS index drift: failed to read file").
			String("object_id", fixCtx.Obj.ID).
			File(filePath).
			WithError(err).
			Log()
		return false, ""
	}
	if len(fileContent) == 0 {
		logging.Fluent(fixCtx.Logger).Warn("Cannot fix CAS index drift: file is empty").
			String("object_id", fixCtx.Obj.ID).
			File(filePath).
			Log()
		return false, ""
	}

	// Kind-from-ID for CAS directory: always write to the correct kind dir (e.g. namespace_registries not namespaces)
	if k := inferKindFromID(fixCtx.Obj.ID); k != emptyValue {
		fixKind = k
	}

	// Resolve kindDir and CAS instance (prefer caller's storage so we use same backend)
	dirName := objects.GetDirectoryFromKind(fixKind)
	if dirName == emptyValue {
		logging.Fluent(fixCtx.Logger).Warn("Cannot fix CAS index drift: unknown kind directory").
			String("object_id", fixCtx.Obj.ID).
			Kind(fixKind).
			Log()
		return false, ""
	}
	kindDir := datacell.CellCASPrimaryDir(projectRoot, dirName)

	var cas *storage.ContentAddressableStorage
	if storageProvider != nil {
		if fileStorage, ok := storageProvider.(*storage.FileObjectStorage); ok {
			if casFromStorage, casErr := fileStorage.GetContentAddressableStorage(fixKind); casErr == nil && casFromStorage != nil {
				cas = casFromStorage
			}
		}
	}
	if cas == nil {
		storageFactory, sfErr := storage.NewStorageFactory(pkgctx.NewSystemContext(), projectRoot)
		if sfErr == nil && storageFactory != nil {
			if sp := storageFactory.GetStorage(); sp != nil {
				if fileStorage, ok := sp.(*storage.FileObjectStorage); ok {
					if casFromStorage, casErr := fileStorage.GetContentAddressableStorage(fixKind); casErr == nil && casFromStorage != nil {
						cas = casFromStorage
					}
				}
			}
		}
	}
	if cas == nil {
		cas = storage.NewContentAddressableStorage(kindDir, fixKind)
	}

	// Compute content hash
	hashBytes := sha256.Sum256(fileContent)
	hash := hex.EncodeToString(hashBytes[:])

	// If the file isn't already the hash-addressed file, ensure the hash-addressed file exists.
	filename := filepath.Base(filePath)
	isHashBased := len(filename) == 69 && strings.HasSuffix(filename, ".yaml") && isHexString(filename[:64])
	if !isHashBased || filename[:64] != hash {
		// This writes the content-addressed file (hash.yaml). Index persistence is handled below.
		if err := cas.Create(fixCtx.Obj.ID, fileContent); err != nil && !strings.Contains(err.Error(), casErrAlreadyExists) {
			logging.Fluent(fixCtx.Logger).Warn("Failed to ensure CAS hash file exists").
				String("object_id", fixCtx.Obj.ID).
				Kind(fixKind).
				String("hash", hash[:16]+"...").
				WithError(err).
				Log()
			return false, ""
		}
	}

	// Force index mapping to the computed content hash and wait for persistence.
	// Use the CAS's own write queue so tests with per-project queues (e.g. SetListingIndexWriteQueueFactoryToPerProjectRoot) work.
	writeQueue := cas.GetWriteQueue()
	done, err := writeQueue.EnqueueUpdateWithCallback(fixKind, fixCtx.Obj.ID, hash, cas)
	if err != nil {
		logging.Fluent(fixCtx.Logger).Warn("Failed to queue CAS index update for drift fix").
			String("object_id", fixCtx.Obj.ID).
			Kind(fixKind).
			String("hash", hash[:16]+"...").
			WithError(err).
			Log()
		return false, ""
	}
	// Wait for callback with timeout so tests and CLI never hang (e.g. queue worker blocked).
	var updateErr error
	select {
	case updateErr = <-done:
	case <-time.After(casFixWaitTimeout):
		logging.Fluent(fixCtx.Logger).Warn("CAS index update did not complete within timeout").
			String("object_id", fixCtx.Obj.ID).
			Kind(fixKind).
			String("hash", hash[:16]+"...").
			Log()
		return false, ""
	}
	if updateErr != nil {
		logging.Fluent(fixCtx.Logger).Warn("CAS index update callback failed for drift fix").
			String("object_id", fixCtx.Obj.ID).
			Kind(fixKind).
			String("hash", hash[:16]+"...").
			WithError(updateErr).
			Log()
		return false, ""
	}

	// Ensure index is persisted (best-effort; callback already implies completion).
	_ = storage.FlushListingIndexForKind(fixKind)

	// If this was an ID-based file, remove the old file now that CAS file is written.
	hashFilePath := filepath.Join(kindDir, hash+".yaml")
	if filePath != hashFilePath {
		if _, statErr := os.Stat(filePath); statErr == nil {
			// Only remove if inside the kind directory tree (defensive).
			oldDir := filepath.Dir(filePath)
			if oldDir == kindDir || strings.HasPrefix(oldDir, kindDir+string(os.PathSeparator)) {
				if rmErr := os.Remove(filePath); rmErr != nil && !os.IsNotExist(rmErr) {
					logging.Fluent(fixCtx.Logger).Warn("Failed to remove legacy file after CAS drift fix (non-fatal)").
						String("object_id", fixCtx.Obj.ID).
						String("old_file", filePath).
						WithError(rmErr).
						Log()
				}
			}
		}
	}

	invalidateValidationCacheForObject(fixCtx, fixCtx.Obj.ID)

	fixedMsg := fmt.Sprintf("Repaired CAS index drift for %s (hash: %s...)", fixCtx.Obj.ID, hash[:16])
	logging.Fluent(fixCtx.Logger).Info("Auto-fixed CAS index drift").
		String("object_id", fixCtx.Obj.ID).
		Kind(fixKind).
		String("hash", hash[:16]+"...").
		Log()
	return true, fixedMsg
}

// fixCASMissingHash fixes missing hash for CAS files by indexing the existing file.
// This handles the case where a file exists but isn't in the CAS index.
// storageProvider should be the same instance used for the check so fixes persist correctly.
func fixCASMissingHash(fixCtx *AutoFixContext, fixKind string, storageProvider storage.ObjectStorageProvider) (bool, string) {
	projectRoot := fixCtx.Ctx.ProjectRoot
	projectRoot = ProjectRootOrResolve(projectRoot)
	if projectRoot == emptyValue {
		logging.Fluent(fixCtx.Logger).Warn("Cannot fix CAS missing hash: no project root").
			String("object_id", fixCtx.Obj.ID).
			Log()
		return false, ""
	}

	// Read the file content - CRITICAL: Must read actual file content.
	// Discovery path may be wrong-kind (e.g. ROL-007 under metrics/) — resolve by ID and use that kind.
	_, fixKind, fileContent, err := resolveCASFixFilePathAndContent(projectRoot, fixCtx, fixKind)
	if err != nil {
		var filePath string // used in fallback resolution below
		// File doesn't exist at expected path - might be in a bucket or already hash-based
		// For missing hash issues, the file should exist but just not be indexed
		// Try to find it by searching bucket directories or using CAS if already indexed
		dirName := objects.GetDirectoryFromKind(fixKind)
		if dirName != emptyValue {
			processDir := datacell.ProcessPrimaryDir(projectRoot)
			kindDir := filepath.Join(processDir, dirName)
			var cas *storage.ContentAddressableStorage
			if c, ok := getCachedCASForKind(pkgctx.NewSystemContext(), projectRoot, fixKind); ok {
				cas = c
			}
			if cas == nil {
				cas = storage.NewContentAddressableStorage(kindDir, fixKind)
			}

			// Check if this is a stale index entry (file doesn't exist anywhere)
			// Try to get file path from CAS (searches buckets) - might already be indexed
			if foundPath, pathErr := cas.GetFilePathForID(fixCtx.Obj.ID); pathErr == nil {
				// Found file in CAS - use that path (object is already indexed, just verify)
				filePath = foundPath
				fileContent, err = os.ReadFile(filePath)
				if err == nil {
					// File found and read - update path and continue
					fixCtx.FilePath = filePath
				}
			}

			// If still not found, search bucket directories for hash-based files
			if err != nil {
				// Search all bucket directories for files matching the object ID
				// This handles cases where file exists but path is wrong
				entries, readErr := os.ReadDir(kindDir)
				if readErr == nil {
					for _, entry := range entries {
						if entry.IsDir() {
							bucketDir := filepath.Join(kindDir, entry.Name())
							// Search for hash-based files in this bucket
							bucketEntries, bucketErr := os.ReadDir(bucketDir)
							if bucketErr == nil {
								for _, bucketEntry := range bucketEntries {
									if !bucketEntry.IsDir() && strings.HasSuffix(bucketEntry.Name(), ".yaml") {
										bucketFile := filepath.Join(bucketDir, bucketEntry.Name())
										// Try to read and check if it contains our object ID
										testContent, testErr := os.ReadFile(bucketFile)
										if testErr == nil {
											var testObj map[string]any
											if yaml.Unmarshal(testContent, &testObj) == nil {
												if objID, ok := testObj[objects.FieldKeyID].(string); ok && objID == fixCtx.Obj.ID {
													// Found it!
													filePath = bucketFile
													fileContent = testContent
													err = nil
													fixCtx.FilePath = filePath
													break
												}
											}
										}
									}
								}
								if err == nil {
									break
								}
							}
						}
					}
				}
			}
		}

		if err != nil {
			// At this point we could not locate the file anywhere on disk. Either (1) stale CAS
			// index mapping (ID -> hash) with hash file deleted, or (2) file was deleted after
			// discovery (e.g. ephemeral audit/metrics). Remove index entry if present; otherwise
			// treat as skipped so we don't repeatedly fail the same object.
			if dirName := objects.GetDirectoryFromKind(fixKind); dirName != emptyValue {
				processDir := datacell.ProcessPrimaryDir(projectRoot)
				kindDir := filepath.Join(processDir, dirName)
				var cas *storage.ContentAddressableStorage
				if c, ok := getCachedCASForKind(pkgctx.NewSystemContext(), projectRoot, fixKind); ok {
					cas = c
				}
				if cas == nil {
					cas = storage.NewContentAddressableStorage(kindDir, fixKind)
				}
				if _, hashErr := cas.GetHashForID(fixCtx.Obj.ID); hashErr == nil {
					if delErr := cas.Delete(fixCtx.Obj.ID); delErr == nil {
						logging.Fluent(fixCtx.Logger).Warn("Removed stale CAS index entry for missing file").
							String("object_id", fixCtx.Obj.ID).
							Kind(fixKind).
							String("expected_file", fixCtx.FilePath).
							String("note", "CAS mapping existed but file could not be found; mapping removed").
							Log()
						return true, fmt.Sprintf("Removed stale CAS index entry for %s (missing file)", fixCtx.Obj.ID)
					}
				}
			}

			// No index entry to remove; file not found (e.g. deleted after discovery). Skip so we
			// don't keep reporting the same violation.
			logging.Fluent(fixCtx.Logger).Info("Skipping CAS missing-hash fix - file not found (nothing to index)").
				String("object_id", fixCtx.Obj.ID).
				String("expected_file", fixCtx.FilePath).
				Kind(fixKind).
				Log()
			return true, fmt.Sprintf("Skipped - file not found for %s (nothing to index)", fixCtx.Obj.ID)
		}
	}

	if len(fileContent) == 0 {
		logging.Fluent(fixCtx.Logger).Warn("File is empty, cannot index").
			String("object_id", fixCtx.Obj.ID).
			File(fixCtx.FilePath).
			Log()
		return false, ""
	}

	// Get kind directory
	dirName := objects.GetDirectoryFromKind(fixKind)
	if dirName == emptyValue {
		logging.Fluent(fixCtx.Logger).Warn("Cannot determine kind directory for CAS indexing").
			String("object_id", fixCtx.Obj.ID).
			Kind(fixKind).
			Log()
		return false, ""
	}

	processDir := datacell.ProcessPrimaryDir(projectRoot)
	kindDir := filepath.Join(processDir, dirName)

	// Prefer caller's storage so we use the same CAS instance and persistence path
	if storageProvider != nil {
		if fileStorage, ok := storageProvider.(*storage.FileObjectStorage); ok {
			cas, err := fileStorage.GetContentAddressableStorage(fixKind)
			if err == nil && cas != nil {
				return fixCASMissingHashWithCAS(fixCtx, fixKind, kindDir, cas)
			}
		}
	}

	// Prefer cached CAS (avoids loading full index per fix)
	if c, ok := getCachedCASForKind(pkgctx.NewSystemContext(), projectRoot, fixKind); ok {
		return fixCASMissingHashWithCAS(fixCtx, fixKind, kindDir, c)
	}

	// Fallback: create factory and get CAS
	storageFactory, err := storage.NewStorageFactory(pkgctx.NewSystemContext(), projectRoot)
	if err != nil {
		logging.Fluent(fixCtx.Logger).Warn("Failed to create storage factory, creating CAS directly").
			String("object_id", fixCtx.Obj.ID).
			WithError(err).
			Log()
		cas := storage.NewContentAddressableStorage(kindDir, fixKind)
		return fixCASMissingHashWithCAS(fixCtx, fixKind, kindDir, cas)
	}

	sp := storageFactory.GetStorage()
	if sp != nil {
		if fileStorage, ok := sp.(*storage.FileObjectStorage); ok {
			cas, casErr := fileStorage.GetContentAddressableStorage(fixKind)
			if casErr == nil && cas != nil {
				return fixCASMissingHashWithCAS(fixCtx, fixKind, kindDir, cas)
			}
		}
	}

	cas := storage.NewContentAddressableStorage(kindDir, fixKind)
	return fixCASMissingHashWithCAS(fixCtx, fixKind, kindDir, cas)
}

// removeStaleCASIndexEntry removes a stale CAS index entry when file doesn't exist
func removeStaleCASIndexEntry(fixCtx *AutoFixContext, cas *storage.ContentAddressableStorage, fixKind, objectID string) (bool, string) {
	logging.Fluent(fixCtx.Logger).Warn("CAS file does not exist, removing stale index entry").
		String("object_id", objectID).
		Kind(fixKind).
		Log()

	// Remove from CAS index
	casIndex := cas.GetIndex()
	if casIndex != nil {
		if removeErr := casIndex.RemoveMapping(objectID); removeErr != nil {
			// If object not in index, that's fine - already cleaned up
			if !strings.Contains(removeErr.Error(), "object not found") && !strings.Contains(removeErr.Error(), "not found") {
				logging.Fluent(fixCtx.Logger).Debug(casMsgRemoveStaleIndexFailed).
					String("object_id", objectID).
					WithError(removeErr).
					Log()
			}
		}
		// Always return success - stale entry removed or already gone
		fixedMsg := fmt.Sprintf("Removed stale CAS index entry for %s (file does not exist)", objectID)
		logging.Fluent(fixCtx.Logger).Info("Removed stale CAS index entry").
			String("object_id", objectID).
			Log()
		return true, fixedMsg
	}
	// No index available - still mark as fixed since file doesn't exist
	fixedMsg := fmt.Sprintf("CAS file does not exist for %s (stale index entry)", objectID)
	return true, fixedMsg
}

// fixCASMissingHashWithCAS performs the actual fix using the provided CAS instance
func fixCASMissingHashWithCAS(fixCtx *AutoFixContext, fixKind, kindDir string, cas *storage.ContentAddressableStorage) (bool, string) {
	// Read the file content first
	// If file doesn't exist at expected path, try to find it via CAS (for bucketed storage)
	filePath := fixCtx.FilePath
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		// File might be in a bucket - try to find it using CAS
		if foundPath, pathErr := cas.GetFilePathForID(fixCtx.Obj.ID); pathErr == nil {
			filePath = foundPath
			fileContent, err = os.ReadFile(filePath)
		}

		if err != nil {
			logging.Fluent(fixCtx.Logger).Warn("Failed to read file for CAS indexing").
				String("object_id", fixCtx.Obj.ID).
				File(fixCtx.FilePath).
				String("searched_path", filePath).
				WithError(err).
				Log()
			return false, ""
		}
		// Update file path to the found location
		fixCtx.FilePath = filePath
	}

	// Check if object is already in index (race condition protection)
	_, indexErr := cas.GetHashForID(fixCtx.Obj.ID)
	if indexErr == nil {
		// Already indexed - this can happen if another process indexed it
		// Verify the hash file exists
		hash, hashErr := cas.GetHashForID(fixCtx.Obj.ID)
		if hashErr == nil {
			hashFile := filepath.Join(kindDir, hash+".yaml")
			if _, err := os.Stat(hashFile); err == nil {
				fixedMsg := fmt.Sprintf("Object %s already indexed in CAS", fixCtx.Obj.ID)
				logging.Fluent(fixCtx.Logger).Info("Object already in CAS index").
					String("object_id", fixCtx.Obj.ID).
					Log()
				return true, fixedMsg
			}
		}
		// Index entry exists but file doesn't - remove stale index entry and re-index
		logging.Fluent(fixCtx.Logger).Debug("Stale index entry detected, re-indexing").
			String("object_id", fixCtx.Obj.ID).
			Log()
		casIndex := cas.GetIndex()
		if casIndex != nil {
			if removeErr := casIndex.RemoveMapping(fixCtx.Obj.ID); removeErr != nil {
				logging.Fluent(fixCtx.Logger).Debug(casMsgRemoveStaleIndexFailed).
					String("object_id", fixCtx.Obj.ID).
					WithError(removeErr).
					Log()
			}
		}
	}

	// Calculate hash from file content
	hashBytes := sha256.Sum256(fileContent)
	hash := hex.EncodeToString(hashBytes[:])

	// Check if file is already hash-based (CAS format)
	filename := filepath.Base(fixCtx.FilePath)
	isHashBased := len(filename) == 69 && strings.HasSuffix(filename, ".yaml") && isHexString(filename[:64])

	if isHashBased {
		// File is already hash-based - verify hash matches filename
		fileHash := filename[:64]
		if fileHash != hash {
			// Hash mismatch - file content doesn't match filename
			// This is a corruption issue, but we can still index it with correct hash
			logging.Fluent(fixCtx.Logger).Warn("Hash-based file has content mismatch, using computed hash").
				String("object_id", fixCtx.Obj.ID).
				String("filename_hash", fileHash[:16]+"...").
				String("computed_hash", hash[:16]+"...").
				Log()
		}

		// Add to index with computed hash (may need to rename file if hash differs)
		// Use write queue for consistency with other CAS operations
		// For auto-fix, we need synchronous behavior, so use callback version
		writeQueue := storage.GetGlobalListingIndexWriteQueue()
		done, err := writeQueue.EnqueueUpdateWithCallback(fixKind, fixCtx.Obj.ID, hash, cas)
		if err != nil {
			logging.Fluent(fixCtx.Logger).Warn("Failed to queue object for CAS index update").
				String("object_id", fixCtx.Obj.ID).
				String("hash", hash[:16]+"...").
				WithError(err).
				Log()
			return false, ""
		}
		// Wait for index update to complete with timeout (avoid hanging tests/CLI).
		var updateErr error
		select {
		case updateErr = <-done:
		case <-time.After(casFixWaitTimeout):
			logging.Fluent(fixCtx.Logger).Warn("CAS index update (missing hash) did not complete within timeout").
				String("object_id", fixCtx.Obj.ID).
				String("hash", hash[:16]+"...").
				Log()
			return false, ""
		}
		if updateErr != nil {
			logging.Fluent(fixCtx.Logger).Warn("Failed to add object to CAS index").
				String("object_id", fixCtx.Obj.ID).
				String("hash", hash[:16]+"...").
				WithError(updateErr).
				Log()
			return false, ""
		}

		// CRITICAL: Flush index updates to ensure index is persisted before returning
		// This is required for tests and ensures the fix is complete
		if err := storage.FlushListingIndexForKind(fixKind); err != nil {
			logging.Fluent(fixCtx.Logger).Warn(casMsgFlushIndexWarn).
				String("object_id", fixCtx.Obj.ID).
				Kind(fixKind).
				WithError(err).
				Log()
			// Don't fail - index update is queued and will be processed
		}

		// If hash differs, rename file to match computed hash
		if fileHash != hash {
			correctHashFile := filepath.Join(filepath.Dir(fixCtx.FilePath), hash+".yaml")
			if err := os.Rename(fixCtx.FilePath, correctHashFile); err != nil {
				logging.Fluent(fixCtx.Logger).Warn("Failed to rename hash file to match computed hash").
					String("object_id", fixCtx.Obj.ID).
					String("old_file", fixCtx.FilePath).
					String("new_file", correctHashFile).
					WithError(err).
					Log()
				// Continue - index is updated, file will be fixed on next update
			}
		}

		// CRITICAL: Invalidate validation cache after indexing
		invalidateValidationCacheForObject(fixCtx, fixCtx.Obj.ID)

		fixedMsg := fmt.Sprintf("Indexed existing CAS file %s (hash: %s)", fixCtx.Obj.ID, hash[:16]+"...")
		logging.Fluent(fixCtx.Logger).Info("Added object to CAS index").
			String("object_id", fixCtx.Obj.ID).
			String("hash", hash[:16]+"...").
			Log()
		return true, fixedMsg
	}

	// File is ID-based - migrate to CAS by creating hash-based file
	// Use CAS.Create() which handles hash calculation, file creation, and index update atomically
	// CRITICAL: CAS.Create() will create the hash file and add to index, even if ID-based file exists
	logging.Fluent(fixCtx.Logger).Info("Migrating ID-based file to CAS").
		String("object_id", fixCtx.Obj.ID).
		File(fixCtx.FilePath).
		String("kind_dir", kindDir).
		Int("content_size", len(fileContent)).
		Log()

	// Calculate expected hash before calling Create (for logging/debugging)
	expectedHash := storage.CalculateSHA256Hash(fileContent)
	logging.Fluent(fixCtx.Logger).Debug("Expected hash for migration").
		String("object_id", fixCtx.Obj.ID).
		String("expected_hash", expectedHash[:16]+"...").
		Log()

	if err := cas.Create(fixCtx.Obj.ID, fileContent); err != nil {
		// Check if error is "object already exists" - might have been created by another process
		if strings.Contains(err.Error(), casErrAlreadyExists) {
			// Verify it's actually indexed now
			_, checkErr := cas.GetHashForID(fixCtx.Obj.ID)
			if checkErr == nil {
				fixedMsg := fmt.Sprintf("Object %s already exists in CAS (indexed by another process)", fixCtx.Obj.ID)
				logging.Fluent(fixCtx.Logger).Info("Object already in CAS").
					String("object_id", fixCtx.Obj.ID).
					Log()
				// CRITICAL: Still invalidate cache even if already exists
				invalidateValidationCacheForObject(fixCtx, fixCtx.Obj.ID)
				return true, fixedMsg
			}
		}

		logging.Fluent(fixCtx.Logger).Error("Failed to create object in CAS (migration)", errfmt.Newf("CAS.Create() failed").Wrap(err)).
			String("object_id", fixCtx.Obj.ID).
			File(fixCtx.FilePath).
			String("kind_dir", kindDir).
			Int("content_size", len(fileContent)).
			String("expected_hash", expectedHash[:16]+"...").
			Log()
		return false, ""
	}

	logging.Fluent(fixCtx.Logger).Info("CAS.Create() succeeded").
		String("object_id", fixCtx.Obj.ID).
		String("expected_hash", expectedHash[:16]+"...").
		Log()

	// CRITICAL: Flush index updates to ensure index is persisted before returning
	// This is required for tests and ensures the fix is complete
	if err := storage.FlushListingIndexForKind(fixKind); err != nil {
		logging.Fluent(fixCtx.Logger).Warn(casMsgFlushIndexWarn).
			String("object_id", fixCtx.Obj.ID).
			Kind(fixKind).
			WithError(err).
			Log()
		// Don't fail - index update is queued and will be processed
	}

	// Verify object is now in index
	newHash, err := cas.GetHashForID(fixCtx.Obj.ID)
	if err != nil {
		// If still not found after flush, try reloading CAS instance (index might be on disk but not in memory)
		cas2 := storage.NewContentAddressableStorage(kindDir, fixKind)
		newHash2, err2 := cas2.GetHashForID(fixCtx.Obj.ID)
		if err2 != nil {
			logging.Fluent(fixCtx.Logger).Error("Object created in CAS but not found in index even after flush and reload", errfmt.Newf("CAS.Create() succeeded but GetHashForID() failed").Wrap(err2)).
				String("object_id", fixCtx.Obj.ID).
				String("kind_dir", kindDir).
				Log()
			return false, ""
		}
		newHash = newHash2
	}

	// CRITICAL: Invalidate validation cache after indexing
	// This ensures the next check sees the object as properly indexed
	invalidateValidationCacheForObject(fixCtx, fixCtx.Obj.ID)

	// Remove old ID-based file if it exists and is different from new hash file
	newHashFile := filepath.Join(kindDir, newHash+".yaml")
	if fixCtx.FilePath != newHashFile {
		// Check if old file still exists (might have been in a bucket)
		if _, err := os.Stat(fixCtx.FilePath); err == nil {
			// Old file exists - remove it (but be careful about bucketed storage)
			oldFileDir := filepath.Dir(fixCtx.FilePath)
			if oldFileDir == kindDir || strings.HasPrefix(oldFileDir, kindDir+"/") {
				// Old file is in kind directory or subdirectory - safe to remove
				if err := os.Remove(fixCtx.FilePath); err != nil && !os.IsNotExist(err) {
					logging.Fluent(fixCtx.Logger).Warn("Failed to remove old ID-based file (non-critical)").
						String("object_id", fixCtx.Obj.ID).
						String("old_file", fixCtx.FilePath).
						WithError(err).
						Log()
					// Don't fail - file is indexed, old file can be cleaned up later
				}
			}
		}
	}

	fixedMsg := fmt.Sprintf("Migrated and indexed object %s to CAS (hash: %s)", fixCtx.Obj.ID, newHash[:16]+"...")
	logging.Fluent(fixCtx.Logger).Info("Migrated object to CAS").
		String("object_id", fixCtx.Obj.ID).
		String("hash", newHash[:16]+"...").
		Log()
	return true, fixedMsg
}
