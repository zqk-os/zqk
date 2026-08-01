package storage

import (
	"os"
	"path/filepath"
	"regexp"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
)

// findHashFile searches for a hash file using bucket key from index (bucket strategy) then base dir then scan.
// bucketKey is from index; when non-empty, try kindDir/bucketKey/hash first.
//
//nolint:gocritic // Multiple return values intentional (path, dir)
func (cas *ContentAddressableStorage) findHashFile(hash string, bucketKey string) (string, string) {
	// First try bucket from index (bucket strategy at create time)
	if bucketKey != emptyValue {
		bucketFile := filepath.Join(cas.kindDir, bucketKey, hash+".yaml")
		if _, err := os.Stat(bucketFile); err == nil {
			return bucketFile, filepath.Join(cas.kindDir, bucketKey)
		}
	}

	// Then try the base directory
	hashFile := filepath.Join(cas.kindDir, hash+".yaml")
	if _, err := os.Stat(hashFile); err == nil {
		return hashFile, cas.kindDir
	}

	// Fallback: search all subdirectories (for legacy or discovery)
	entries, err := os.ReadDir(cas.kindDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				bucketFile := filepath.Join(cas.kindDir, entry.Name(), hash+".yaml")
				if _, err := os.Stat(bucketFile); err == nil {
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
	if _, err := os.Stat(path); err == nil {
		var err_swallow_1 = os.Remove(path)
		if err_swallow_1 !=

			// Subdirs (buckets)
			nil {
			logging.LogSwallowedError(err_swallow_1)
		}
		return
	}

	entries, err := os.ReadDir(cas.kindDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path = filepath.Join(cas.kindDir, e.Name(), fileName)
		if _, err := os.Stat(path); err == nil {
			var err_swallow_2 = os.Remove(path)
			if err_swallow_2 != nil {
				logging.

					// writeFileWithSync writes a file atomically with sync
					LogSwallowedError(err_swallow_2)
			}
			return
		}
	}
}

func (cas *ContentAddressableStorage) writeFileWithSync(filePath string, data []byte) error {
	// Hash files are content-addressed and should be immutable once created.
	// We therefore avoid truncating/overwriting existing files and use a safe create flow.
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, paths.DirPerm755); err != nil {
		return errfmt.Newf(ErrMsgCreateDir).Wrap(err)
	}

	// Fast-path: file already exists. Verify it matches the expected hash for integrity.
	if existing, err := os.ReadFile(filePath); err == nil {
		expectedHash := CalculateSHA256Hash(data)
		actualHash := CalculateSHA256Hash(existing)
		if actualHash != expectedHash {
			return errfmt.Errorf(ConstMiscExistingCasFileHashMismatchExpectedSGotS, expectedHash, actualHash)
		}
		return nil
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(filePath)+".tmp-*")
	if err != nil {
		return errfmt.Newf(ConstMiscFailedToCreateTempFile).Wrap(err)
	}
	tmpName := tmp.Name()
	defer func() {
		var err_swallow_3 = tmp.Close()
		if err_swallow_3 != nil {
			logging.LogSwallowedError(err_swallow_3)
		}
		var err_swallow_4 = os.Remove(tmpName)
		if err_swallow_4 != nil {
			logging.LogSwallowedError(err_swallow_4)
		}
	}()

	if err := os.Chmod(tmpName, paths.FilePerm644); err != nil && !os.IsNotExist(err) {
		return errfmt.Newf(ConstMiscFailedToChmodTempFile).Wrap(err)
	}

	if _, err := tmp.Write(data); err != nil {
		return errfmt.Newf(ConstMiscFailedToWriteTempFile).Wrap(err)
	}
	// Do NOT call tmp.Sync() here. On macOS, Sync() triggers F_FULLFSYNC which can stall
	// 1-2 seconds per call. For 327 bulk CAS creates (scan-tests --all) that becomes
	// 5-10 minutes of blocking. The atomic hardlink below provides sufficient atomicity;
	// APFS journaling handles crash safety without requiring application-level fsync.
	if err := tmp.Close(); err != nil {
		return errfmt.Newf(ConstMiscFailedToCloseTempFile).Wrap(err)
	}

	// Atomically publish without overwriting: hardlink temp -> final.
	// This avoids clobbering an existing file created concurrently by another process.
	for {
		if err := os.Link(tmpName, filePath); err != nil {
			if os.IsExist(err) {
				// Another process won the race. Verify existing matches what we intended.
				existing, readErr := os.ReadFile(filePath)
				if readErr != nil {
					if os.IsNotExist(readErr) {
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

	// Directory sync intentionally omitted: same reason as temp-file Sync above.
	// APFS journaling makes directory-entry changes durable without application fsync.

	return nil
}

// GetHashForID returns the hash for a given object ID, with lazy auto-healing for index lags
func (cas *ContentAddressableStorage) GetHashForID(objectID string) (string, error) {
	hash, err := cas.index.GetHash(objectID)
	if err != nil {
		// ITEM-EXAMPLE: Lazily auto-heal the index upon detection.
		// Since missing index entries for existing valid hashed files is a known
		// non-destructive indexing race condition inherent to our architecture,
		// we auto-heal the index lag by scanning for the file and updating the index.
		_, discoveredHash, scanErr := discoverCASFilePathByScanning(objectID, cas.kindDir)
		if scanErr == nil {
			// Heal the in-memory index
			cas.setIndexMappingInMemory(objectID, discoveredHash, "")

			// Queue the index update asynchronously
			writeQueue := cas.getWriteQueue()
			opCallback := cas.getOperationCallback()
			if getSkipIndexUpdateWait() {
				_, _ = writeQueue.enqueue(cas.kind, objectID, discoveredHash, "", "", cas, opCallback, false, false)
			} else {
				_, _ = writeQueue.EnqueueUpdateWithOperationCallback(cas.kind, objectID, discoveredHash, "", cas, opCallback)
			}
			return discoveredHash, nil
		}
		// Return original error if discovery fails
	}
	return hash, err
}

// GetFilePathForID returns the file path for an object ID using the bucket key from the index (bucket strategy).
// When the index has a bucket key for this ID, path is kindDir/bucketKey/hash.yaml; otherwise kindDir/hash.yaml.
func (cas *ContentAddressableStorage) GetFilePathForID(objectID string) (string, error) {
	hash, err := cas.GetHashForID(objectID)
	if err != nil {
		return "", err
	}

	bucketKey := cas.index.GetBucketKey(objectID)

	// Use bucket key from index first (from bucket strategy at create time)
	if bucketKey != emptyValue {
		bucketFile := filepath.Join(cas.kindDir, bucketKey, hash+".yaml")
		if _, err := os.Stat(bucketFile); err == nil {
			return bucketFile, nil
		}
	}

	// Then try the base kindDir (for non-bucketed or flat storage)
	hashFile := filepath.Join(cas.kindDir, hash+".yaml")
	if _, err := os.Stat(hashFile); err == nil {
		return hashFile, nil
	}

	// Fallback: search subdirectories (date-style buckets for legacy)
	entries, err := os.ReadDir(cas.kindDir)
	if err == nil {
		datePattern := regexp.MustCompile(`^\d{4}-\d{2}(-\d{2})?$`)
		for _, entry := range entries {
			if entry.IsDir() && datePattern.MatchString(entry.Name()) {
				bucketFile := filepath.Join(cas.kindDir, entry.Name(), hash+".yaml")
				if _, err := os.Stat(bucketFile); err == nil {
					return bucketFile, nil
				}
			}
		}
	}

	return "", errfmt.Errorf(ConstMiscHashFileNotFoundForIdSHashS, objectID, hash)
}

// ListIDs returns all object IDs in the index
func (cas *ContentAddressableStorage) ListIDs() ([]string, error) {
	return cas.index.ListIDs(), nil
}

// GetAllMappings returns a copy of all ID -> hash mappings
func (cas *ContentAddressableStorage) GetAllMappings() (map[string]string, error) {
	return cas.index.SnapshotMappings(), nil
}
