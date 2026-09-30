package filecas

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
)

// findHashFile searches for a hash file using bucket key from index (bucket strategy) then base dir then scan.
// bucketKey is from index; when non-empty, try kindDir/bucketKey/hash first.
//
//nolint:gocritic // Multiple return values intentional (path, dir)
func (cas *ContentAddressableStorage) findHashFile(hash string, bucketKey string) (string, string) {
	// First try bucket from index (bucket strategy at create time)
	if bucketKey != emptyValue {
		bucketFile := filepath.Join(cas.kindDir, bucketKey, hash+".yaml")
		if _, err := fileutil.Stat(bucketFile); err == nil {
			return bucketFile, filepath.Join(cas.kindDir, bucketKey)
		}
	}

	// Then try the base directory
	hashFile := filepath.Join(cas.kindDir, hash+".yaml")
	if _, err := fileutil.Stat(hashFile); err == nil {
		return hashFile, cas.kindDir
	}

	// Fallback: search all subdirectories (for legacy or discovery)
	entries, err := fileutil.ReadDir(cas.kindDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				bucketFile := filepath.Join(cas.kindDir, entry.Name(), hash+".yaml")
				if _, err := fileutil.Stat(bucketFile); err == nil {
					return bucketFile, filepath.Join(cas.kindDir, entry.Name())
				}
			}
		}
	}

	return "", ""
}

// removeHashFileByScan removes a hash-named file (hash.yaml) from kindDir or any subdir.
// Used when the primary path (from index bucket key) did not exist so the file might be in a bucket.
func (cas *ContentAddressableStorage) removeHashFileByScan(hash string) {
	fileName := hash + ".yaml"
	// Base dir
	path := filepath.Join(cas.kindDir, fileName)
	if _, err := fileutil.Stat(path); err == nil {
		logging.LogSwallowedError(fileutil.RemoveFile(path))
		return
	}

	entries, err := fileutil.ReadDir(cas.kindDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path = filepath.Join(cas.kindDir, e.Name(), fileName)
		if _, err := fileutil.Stat(path); err == nil {
			var err_swallow_2 = fileutil.RemoveFile(path)
			if err_swallow_2 != nil {
				logging.

					// WriteFileWithSync writes a file atomically with sync
					LogSwallowedError(err_swallow_2)
			}
			return
		}
	}
}

func (cas *ContentAddressableStorage) WriteFileWithSync(filePath string, data []byte) error {
	// Hash files are content-addressed and should be immutable once created.
	// We therefore avoid truncating/overwriting existing files and use a safe create flow.
	dir := filepath.Dir(filePath)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		return errfmt.Newf(ErrMsgCreateDir).Wrap(err)
	}

	// Fast-path: file already exists. Verify it matches the expected hash for integrity.
	if existing, err := fileutil.ReadFile(filePath); err == nil {
		expectedHash := CalculateSHA256Hash(data)
		actualHash := CalculateSHA256Hash(existing)
		if actualHash != expectedHash {
			return errfmt.Errorf(ConstMiscExistingCasFileHashMismatchExpectedSGotS, expectedHash, actualHash)
		}
		return nil
	}

	tmp, err := fileutil.CreateTemp(dir, filepath.Base(filePath)+".tmp-*")
	if err != nil {
		return errfmt.Newf(ConstMiscFailedToCreateTempFile).Wrap(err)
	}
	tmpName := tmp.Name()
	defer func() {
		logging.LogSwallowedError(tmp.Close())
		logging.LogSwallowedError(fileutil.RemoveFile(tmpName))
	}()

	if err := fileutil.Chmod(tmpName, paths.FilePerm644); err != nil && !fileutil.IsNotExist(err) {
		return errfmt.Newf(ConstMiscFailedToChmodTempFile).Wrap(err)
	}

	if _, err := tmp.Write(data); err != nil {
		return errfmt.Newf(ConstMiscFailedToWriteTempFile).Wrap(err)
	}
	// Per-OS durability: Darwin queues file fsync to avoid blocking publication;
	// other platforms fsync synchronously.
	if err := CasPublishSyncFile(tmp); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return errfmt.Newf(ConstMiscFailedToCloseTempFile).Wrap(err)
	}

	// Atomically publish without overwriting: hardlink temp -> final.
	// This avoids clobbering an existing file created concurrently by another process.
	for {
		if err := fileutil.Link(tmpName, filePath); err != nil {
			if fileutil.IsExist(err) {
				// Another process won the race. Verify existing matches what we intended.
				existing, readErr := fileutil.ReadFile(filePath)
				if readErr != nil {
					if fileutil.IsNotExist(readErr) {
						// The other process deleted it before we could read. Try linking again.
						continue
					}
					return errfmt.Newf(ConstMiscFailedToReadCasFileAfterLinkRace).Wrap(readErr)
				}
				expectedHash := CalculateSHA256Hash(data)
				actualHash := CalculateSHA256Hash(existing)
				if actualHash != expectedHash {
					return errfmt.Errorf(ConstMiscExistingCasFileHashMismatchAfterLinkRace, expectedHash, actualHash)
				}
				return nil
			}
			return errfmt.Newf(ConstMiscFailedToLinkTempFileIntoPlace).Wrap(err)
		}
		break
	}

	// Persist the published directory entry after the hardlink.
	if err := CasPublishSyncDir(dir); err != nil {
		return err
	}

	return nil
}

// GetHashForID returns the hash for a given object ID, with lazy auto-healing for index lags.
// Last-resort directory scans are singleflighted per ID. Kind-level scan recency must
// not skip that scan: Create can write a bucketed blob after a kind scan.
func (cas *ContentAddressableStorage) GetHashForID(objectID string) (string, error) {
	if strings.TrimSpace(objectID) == emptyValue {
		return "", errfmt.Errorf("empty object ID")
	}

	hash, err := cas.index.GetHash(objectID)
	if err == nil {
		return hash, nil
	}

	// If the index is unpopulated (0 mappings), populate it in bulk first
	if cas.index.Len() == 0 {
		_ = cas.EnsureIndexPopulated()
		if hash, err = cas.index.GetHash(objectID); err == nil {
			return hash, nil
		}
	}

	if GetCASPendingVisibilityCache != nil {
		pendingCache := GetCASPendingVisibilityCache(projectRootFromCASKindDir(cas.kindDir))
		if pendingCache != nil {
			if pending, found := pendingCache.LookupPending(objectID); found && (pending.Kind == "" || pending.Kind == cas.kind) {
				cas.SetIndexMappingInMemory(objectID, pending.Hash, pending.BucketKey)
				return pending.Hash, nil
			}
		}
	}

	// Fast rejection: known misses only. Kind-level lastScanTime must not
	// imply this ID is absent — Create can write a bucketed blob after a
	// kind scan (scheduler_job / qa_success), then proveCreateVisibility
	// would fail with "not found in index" while the YAML is on disk.
	// TRACK: follow-up in kernel backlog
	if cas.isNegativeMiss(objectID) {
		return "", err
	}

	// LAST RESORT: Discover file by scanning, coalesced by singleflight
	res, scanErr, _ := cas.scanGroup.Do("discover_"+objectID, func() (any, error) {
		// Re-check after acquiring singleflight
		if h, checkErr := cas.index.GetHash(objectID); checkErr == nil {
			return h, nil
		}
		if cas.isNegativeMiss(objectID) {
			return "", errfmt.Errorf(ConstStreamIdStrNotFoundInCasDirectoryStr, objectID, cas.kindDir)
		}

		cas.lastScanTime.Store(time.Now().UnixNano())

		discoveredHash, discErr := cas.scanForObjectID(objectID)
		if discErr != nil {
			cas.recordNegativeMiss(objectID)
			return "", discErr
		}
		return discoveredHash, nil
	})

	if scanErr == nil {
		return res.(string), nil
	}
	return "", err
}

func (cas *ContentAddressableStorage) scanForObjectID(targetID string) (string, error) {
	if targetID == emptyValue || cas == nil || cas.kindDir == emptyValue {
		return "", errfmt.Errorf("empty object ID or kind directory")
	}

	var foundHash string
	var foundBucket string

	scanDir := func(dir, bucketKey string) {
		entries, err := fileutil.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !CasHashFilenameRe.MatchString(name) {
				continue
			}
			path := filepath.Join(dir, name)
			id := CasHashFilePeekObjectID(path)
			if id == emptyValue {
				continue
			}
			hash := strings.TrimSuffix(name, filepath.Ext(name))
			// Always populate every peeked file into the in-memory index so disk reads are never wasted
			cas.SetIndexMappingInMemory(id, hash, bucketKey)
			if id == targetID && foundHash == emptyValue {
				foundHash = hash
				foundBucket = bucketKey
			}
		}
	}

	scanDir(cas.kindDir, "")
	if foundHash == emptyValue {
		entries, err := fileutil.ReadDir(cas.kindDir)
		if err == nil {
			for _, e := range entries {
				if e.IsDir() {
					scanDir(filepath.Join(cas.kindDir, e.Name()), e.Name())
					if foundHash != emptyValue {
						break
					}
				}
			}
		}
	}

	if foundHash != emptyValue {
		writeQueue := cas.getWriteQueue()
		if writeQueue != nil {
			opCallback := cas.getOperationCallback()
			if GetSkipIndexUpdateWait() {
				_, _ = writeQueue.EnqueueInternal(cas.kind, targetID, foundHash, foundBucket, "", cas, opCallback, false, false)
			} else {
				_, _ = writeQueue.EnqueueUpdateWithOperationCallback(cas.kind, targetID, foundHash, foundBucket, cas, opCallback)
			}
		}
		return foundHash, nil
	}

	return "", errfmt.Errorf(ConstStreamIdStrNotFoundInCasDirectoryStr, targetID, cas.kindDir)
}

// GetFilePathForID returns the file path for an object ID using the bucket key from the index (bucket strategy).
// When the index has a bucket key for this ID, path is kindDir/bucketKey/hash.yaml; otherwise kindDir/hash.yaml.
// If the index maps to a missing blob, the ghost mapping is dropped durably and a miss is returned.
// TRACK: follow-up in kernel backlog
func (cas *ContentAddressableStorage) GetFilePathForID(objectID string) (string, error) {
	hash, err := cas.GetHashForID(objectID)
	if err != nil {
		return "", err
	}

	bucketKey := cas.index.GetBucketKey(objectID)

	// Use bucket key from index first (from bucket strategy at create time)
	if bucketKey != emptyValue {
		bucketFile := filepath.Join(cas.kindDir, bucketKey, hash+".yaml")
		if _, err := fileutil.Stat(bucketFile); err == nil {
			return bucketFile, nil
		}
	}

	// Then try the base kindDir (for non-bucketed or flat storage)
	hashFile := filepath.Join(cas.kindDir, hash+".yaml")
	if _, err := fileutil.Stat(hashFile); err == nil {
		return hashFile, nil
	}

	// Fallback: search subdirectories (date-style buckets for legacy)
	entries, err := fileutil.ReadDir(cas.kindDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() && casDateBucketNameRe.MatchString(entry.Name()) {
				bucketFile := filepath.Join(cas.kindDir, entry.Name(), hash+".yaml")
				if _, err := fileutil.Stat(bucketFile); err == nil {
					return bucketFile, nil
				}
			}
		}
	}

	// Ghost mapping: index points at a deleted hash blob — drop it so warm/check cannot flap.
	// Protect recently created/updated objects from being dropped during transient filesystem races.
	pendingCache := GetCASPendingVisibilityCache(projectRootFromCASKindDir(cas.kindDir))
	if pendingCache != nil {
		if _, found := pendingCache.LookupPending(objectID); found {
			return "", errfmt.Errorf(ConstMiscHashFileNotFoundForIdSHashS, objectID, hash)
		}
	}
	cas.dropGhostIndexMapping(objectID)
	return "", errfmt.Errorf(ConstMiscHashFileNotFoundForIdSHashS, objectID, hash)
}

// dropGhostIndexMapping removes a stale id→hash mapping when the blob is gone.
func (cas *ContentAddressableStorage) dropGhostIndexMapping(objectID string) {
	if objectID == emptyValue || cas.index == nil {
		return
	}
	cas.removeIndexMappingInMemory(objectID)
	writeQueue := cas.getWriteQueue()
	if writeQueue == nil {
		return
	}
	done, err := writeQueue.EnqueueRemove(cas.kind, objectID, cas)
	if err != nil {
		return
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}
}

// ListIDs returns all object IDs in the index
func (cas *ContentAddressableStorage) ListIDs() ([]string, error) {
	return cas.index.ListIDs(), nil
}

// GetAllMappings returns a copy of all ID -> hash mappings
func (cas *ContentAddressableStorage) GetAllMappings() (map[string]string, error) {
	return cas.index.SnapshotMappings(), nil
}
