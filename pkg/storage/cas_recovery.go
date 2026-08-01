package storage

import (
	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/paths"

	"context"
	"fmt"
	"os"
	"path/filepath"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"gopkg.in/yaml.v3"
)

// CASRecoveryResult represents the result of a CAS recovery operation
type CASRecoveryResult struct {
	ObjectID string
	Kind     string
	Action   string // "recreated", ConstStreamRemovedFromIndex, "skipped"
	Success  bool
	Error    error
	Message  string
}

// CASRecoveryOptions configures CAS recovery behavior
type CASRecoveryOptions struct {
	// RemoveFromIndexIfMissing removes objects from index if file doesn't exist and can't be recovered
	RemoveFromIndexIfMissing bool
	// RecreateFromIDBased attempts to recreate files from ID-based file if it exists
	RecreateFromIDBased bool
	// Logger for logging recovery operations
	Logger logging.Logger
}

// DefaultCASRecoveryOptions returns default recovery options
func DefaultCASRecoveryOptions(logger logging.Logger) *CASRecoveryOptions {
	return &CASRecoveryOptions{
		RemoveFromIndexIfMissing: true,
		RecreateFromIDBased:      true,
		Logger:                   logger,
	}
}

// RecoverCASObject attempts to recover a CAS object that has index entry but missing file
// Returns recovery result indicating what action was taken.
// sharedCAS is optional (INDEX_FIRST_LOW_CPU_SCAN_DESIGN): when non-nil, reuse it to avoid repeated index load.
func RecoverCASObject(
	ctx context.Context,
	projectRoot string,
	objectID string,
	kind string,
	options *CASRecoveryOptions,
	sharedCAS *ContentAddressableStorage,
) (*CASRecoveryResult, error) {
	if options == nil {
		options = &CASRecoveryOptions{
			RemoveFromIndexIfMissing: true,
			RecreateFromIDBased:      true,
			Logger:                   nil, // No logger by default
		}
	}

	result := &CASRecoveryResult{
		ObjectID: objectID,
		Kind:     kind,
		Action:   "skipped",
		Success:  false,
	}

	dirName := objects.GetDirectoryFromKind(kind)
	if dirName == emptyValue {
		result.Error = errfmt.Errorf(ConstStreamUnknownObjectKindStr, kind)
		return result, result.Error
	}

	var kindDir string
	if rel := paths.GetPathAlias(projectRoot, dirName); rel != emptyValue {
		kindDir = filepath.Join(projectRoot, rel)
	} else {
		processDir := datacell.ProcessPrimaryDir(projectRoot)
		kindDir = filepath.Join(processDir, dirName)
	}
	var cas *ContentAddressableStorage
	if sharedCAS != nil {
		cas = sharedCAS
	} else {
		cas = NewContentAddressableStorage(kindDir, kind)
	}

	// Check if object exists in index
	hash, err := cas.GetHashForID(objectID)
	if err != nil {
		// Object not in index - nothing to recover
		result.Message = ConstStreamObjectNotInCasIndex
		return result, nil
	}

	// Check if file exists
	hashFile := filepath.Join(kindDir, hash+".yaml")
	if _, err := os.Stat(hashFile); err == nil {
		// File exists - nothing to recover
		result.Message = ConstStreamFileExistsNoRecoveryNeeded
		result.Success = true
		return result, nil
	}

	// File doesn't exist - try to recover
	if options.RecreateFromIDBased {
		// Try to read from ID-based file
		idBasedFile := filepath.Join(kindDir, fmt.Sprintf("%s.yaml", objectID))
		if data, err := os.ReadFile(idBasedFile); err == nil {
			// ID-based file exists - recreate in CAS
			var obj map[string]any
			if err := yaml.Unmarshal(data, &obj); err != nil {
				result.Error = errfmt.Newf(ConstStreamFailedToParseIdBasedFile).Wrap(err)
				return result, result.Error
			}

			// Calculate hash from content
			normalizedData, err := yaml.Marshal(obj)
			if err != nil {
				result.Error = errfmt.Newf(ConstStreamFailedToMarshalObject).Wrap(err)
				return result, result.Error
			}

			calculatedHash := CalculateSHA256Hash(normalizedData)

			// If hash matches, file was just in wrong location - move it
			if calculatedHash == hash {
				// Hash matches - file should exist but doesn't
				// This shouldn't happen, but try to write it
				if err := cas.writeFileWithSync(hashFile, normalizedData); err != nil {
					result.Error = errfmt.Newf(ConstStreamFailedToRecreateFile).Wrap(err)
					return result, result.Error
				}

				result.Action = "recreated"
				result.Success = true
				result.Message = fmt.Sprintf(ConstStreamRecreatedFileFromIdBasedSourceHashStr, hash)
				if options.Logger != nil {
					StorageLog(options.Logger).Info(LogEventStorageCASRecoveryRecoveredFromIDBased).
						ObjectID(objectID).
						Kind(kind).
						String("hash", hash).
						Log()
				}
				return result, nil
			}

			// Hash doesn't match - content changed
			// Update index with new hash
			writeQueue := GetGlobalListingIndexWriteQueue()
			if err := writeQueue.EnqueueUpdate(kind, objectID, calculatedHash, cas); err != nil {
				result.Error = errfmt.Newf(ConstStreamFailedToUpdateIndex).Wrap(err)
				return result, result.Error
			}

			// Write new file
			newHashFile := filepath.Join(kindDir, calculatedHash+".yaml")
			if err := cas.writeFileWithSync(newHashFile, normalizedData); err != nil {
				result.Error = errfmt.Newf(ConstStreamFailedToWriteNewFile).Wrap(err)
				return result, result.Error
			}

			result.Action = "recreated"
			result.Success = true
			result.Message = fmt.Sprintf(ConstStreamRecreatedFileWithUpdatedHashOldStrNewStr, hash, calculatedHash)
			if options.Logger != nil {
				StorageLog(options.Logger).Info(LogEventStorageCASRecoveryRecoveredHashUpdate).
					ObjectID(objectID).
					Kind(kind).
					String("old_hash", hash).
					String("new_hash", calculatedHash).
					Log()
			}
			return result, nil
		}
	}

	// Can't recover - remove from index if configured
	if options.RemoveFromIndexIfMissing {
		index := cas.GetIndex()
		if index != nil {
			if err := index.RemoveMapping(objectID); err != nil {
				result.Error = errfmt.Newf(ConstStreamFailedToRemoveFromIndex).Wrap(err)
				return result, result.Error
			}

			// Save the index after removal
			if err := index.Save(); err != nil {
				result.Error = errfmt.Newf(ConstStreamFailedToSaveIndexAfterRemoval).Wrap(err)
				return result, result.Error
			}

			result.Action = ConstStreamRemovedFromIndex
			result.Success = true
			result.Message = fmt.Sprintf(ConstStreamRemovedFromIndexFileMissingHashStr, hash)
			if options.Logger != nil {
				StorageLog(options.Logger).Info(LogEventStorageCASRecoveryRemovedFromIndex).
					ObjectID(objectID).
					Kind(kind).
					String("hash", hash).
					Log()
			}
			return result, nil
		}
	}

	// Can't recover and can't remove
	result.Error = errfmt.Errorf(ConstStreamFileMissingAndRecoveryNotPossibleHashStr, hash)
	result.Message = ConstStreamFileMissingRecoveryNotPossible
	return result, result.Error
}

// RecoverCASKind recovers all objects of a kind that have index entries but missing files.
// storageForKind is optional (INDEX_FIRST_LOW_CPU_SCAN_DESIGN): when *FileObjectStorage, reuses its CAS cache to avoid repeated index load.
func RecoverCASKind(
	ctx context.Context,
	projectRoot string,
	kind string,
	options *CASRecoveryOptions,
	storageForKind ObjectStorageProvider,
) ([]*CASRecoveryResult, error) {
	if options == nil {
		options = &CASRecoveryOptions{
			RemoveFromIndexIfMissing: true,
			RecreateFromIDBased:      true,
			Logger:                   nil, // No logger by default
		}
	}

	dirName := objects.GetDirectoryFromKind(kind)
	if dirName == emptyValue {
		return nil, errfmt.Errorf(ConstStreamUnknownObjectKindStr, kind)
	}

	var kindDir string
	if rel := paths.GetPathAlias(projectRoot, dirName); rel != emptyValue {
		kindDir = filepath.Join(projectRoot, rel)
	} else {
		processDir := datacell.ProcessPrimaryDir(projectRoot)
		kindDir = filepath.Join(processDir, dirName)
	}

	var cas *ContentAddressableStorage
	var allMappings map[string]string
	var err error
	if fileStorage, ok := storageForKind.(*FileObjectStorage); ok && fileStorage != nil {
		cas, err = fileStorage.getContentAddressableStorage(kind)
		if err == nil && cas != nil {
			allMappings, err = cas.GetAllMappings()
		}
	}
	if allMappings == nil || err != nil {
		cas = NewContentAddressableStorage(kindDir, kind)
		allMappings, err = cas.GetAllMappings()
	}
	if err != nil {
		return nil, errfmt.Newf(ConstStreamFailedToGetAllMappings).Wrap(err)
	}

	var results []*CASRecoveryResult
	const abortCheckInterval = 1000 // check ctx every N entries so long runs can abort (e.g. SCH-002)
	n := 0
	for objectID, hash := range allMappings {
		if n > 0 && n%abortCheckInterval == 0 && ctx.Err() != nil {
			return results, ctx.Err()
		}
		n++
		hashFile := filepath.Join(kindDir, hash+".yaml")
		if _, err := os.Stat(hashFile); err != nil {
			// File missing - attempt recovery (pass shared CAS to avoid repeated index load)
			result, err := RecoverCASObject(ctx, projectRoot, objectID, kind, options, cas)
			if err != nil {
				// Log error but continue with other objects
				if options.Logger != nil {
					StorageLog(options.Logger).Warn(LogEventStorageCASRecoveryFailed).
						ObjectID(objectID).
						Kind(kind).
						WithError(err).
						Log()
				}
			}
			results = append(results, result)
		}
	}

	return results, nil
}

// ValidateCASIndex validates CAS index entries and returns objects with issues
func ValidateCASIndex(projectRoot string, kind string) ([]*CASRecoveryResult, error) {
	// Get CAS instance
	dirName := objects.GetDirectoryFromKind(kind)
	if dirName == emptyValue {
		return nil, errfmt.Errorf(ConstStreamUnknownObjectKindStr, kind)
	}

	var kindDir string
	if rel := paths.GetPathAlias(projectRoot, dirName); rel != emptyValue {
		kindDir = filepath.Join(projectRoot, rel)
	} else {
		processDir := datacell.ProcessPrimaryDir(projectRoot)
		kindDir = filepath.Join(processDir, dirName)
	}
	cas := NewContentAddressableStorage(kindDir, kind)

	// Get all mappings from index
	allMappings, err := cas.GetAllMappings()
	if err != nil {
		return nil, errfmt.Newf(ConstStreamFailedToGetAllMappings).Wrap(err)
	}

	var issues []*CASRecoveryResult

	// Check each object
	for objectID, hash := range allMappings {
		hashFile := filepath.Join(kindDir, hash+".yaml")
		if _, err := os.Stat(hashFile); err != nil {
			// File missing
			issues = append(issues, &CASRecoveryResult{
				ObjectID: objectID,
				Kind:     kind,
				Action:   "validation",
				Success:  false,
				Message:  fmt.Sprintf(ConstStreamFileMissingForHashStr, hash),
			})
		}
	}

	return issues, nil
}

// RecoverCASObjectViaStorage recovers a CAS object using FileObjectStorage
// This provides access to ID-based file lookup and validation
func (f *FileObjectStorage) RecoverCASObjectViaStorage(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	objectID string,
	options *CASRecoveryOptions,
) (*CASRecoveryResult, error) {
	// Infer kind from ID
	if err := f.idValidator.LoadPatterns(); err != nil {
		return nil, errfmt.Newf(ConstStreamFailedToLoadIdPatterns).Wrap(err)
	}
	kind := f.idValidator.InferKindFromID(objectID)
	if kind == emptyValue {
		return nil, errfmt.Errorf(ConstStreamCouldNotInferKindFromIdStr, objectID)
	}

	// Check if this kind uses CAS
	if !f.usesContentAddressableStorage(kind) {
		return nil, errfmt.Errorf(ConstStreamKindStrDoesNotUseContentAddressableStorage, kind)
	}

	// Use the recovery function with project root
	projectRoot := f.projectRoot
	if projectRoot == emptyValue {
		return nil, errfmt.Errorf(ConstStreamProjectRootNotSet)
	}

	return RecoverCASObject(ctx, projectRoot, objectID, kind, options, nil)
}
