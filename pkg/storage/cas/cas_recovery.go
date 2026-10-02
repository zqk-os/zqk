package cas

import (
	"crypto/sha256"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage/filecas"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"context"
	"fmt"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
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
	EnqueueIndexUpdate func(kind, objectID, hash string, cas *filecas.ContentAddressableStorage) error
	Logger             logging.Logger
}

// DefaultCASRecoveryOptions returns default recovery options
func DefaultCASRecoveryOptions(logger logging.Logger) *CASRecoveryOptions {
	return &CASRecoveryOptions{
		RemoveFromIndexIfMissing: true,
		RecreateFromIDBased:      true,
		Logger:                   logger,
	}
}

func resolveKindDir(projectRoot, kind string) (string, error) {
	dirName := objects.GetDirectoryFromKind(kind)
	if dirName == "" {
		return "", errfmt.Errorf(ConstStreamUnknownObjectKindStr, kind)
	}
	if rel := paths.GetPathAlias(projectRoot, dirName); rel != "" {
		return filepath.Join(projectRoot, rel), nil
	}
	return filepath.Join(datacell.ProcessPrimaryDir(projectRoot), dirName), nil
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
	sharedCAS *filecas.ContentAddressableStorage,
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

	kindDir, err := resolveKindDir(projectRoot, kind)
	if err != nil {
		result.Error = err
		return result, result.Error
	}
	var cas *filecas.ContentAddressableStorage
	if sharedCAS != nil {
		cas = sharedCAS
	} else {
		cas = filecas.NewContentAddressableStorage(kindDir, kind)
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
	if _, err := fileutil.Stat(hashFile); err == nil {
		// File exists - nothing to recover
		result.Message = ConstStreamFileExistsNoRecoveryNeeded
		result.Success = true
		return result, nil
	}

	// File doesn't exist - try to recover
	if options.RecreateFromIDBased {
		// Try to read from ID-based file
		idBasedFile := filepath.Join(kindDir, fmt.Sprintf("%s.yaml", objectID))
		if data, err := fileutil.ReadFile(idBasedFile); err == nil {
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

			calculatedHash := calculateSHA256Hash(normalizedData)

			// If hash matches, file was just in wrong location - move it
			if calculatedHash == hash {
				// Hash matches - file should exist but doesn't
				// This shouldn't happen, but try to write it
				if err := cas.WriteFileWithSync(hashFile, normalizedData); err != nil {
					result.Error = errfmt.Newf(ConstStreamFailedToRecreateFile).Wrap(err)
					return result, result.Error
				}

				result.Action = "recreated"
				result.Success = true
				result.Message = fmt.Sprintf(ConstStreamRecreatedFileFromIdBasedSourceHashStr, hash)
				if options.Logger != nil {
					options.Logger.Info("cas_recovered")
				}
				return result, nil
			}

			// Hash doesn't match - content changed
			// Update index with new hash

			if options.EnqueueIndexUpdate != nil {
				if err := options.EnqueueIndexUpdate(kind, objectID, calculatedHash, cas); err != nil {
					result.Error = errfmt.Newf(ConstStreamFailedToUpdateIndex).Wrap(err)
					return result, result.Error
				}
			}

			// Write new file
			newHashFile := filepath.Join(kindDir, calculatedHash+".yaml")
			if err := cas.WriteFileWithSync(newHashFile, normalizedData); err != nil {
				result.Error = errfmt.Newf(ConstStreamFailedToWriteNewFile).Wrap(err)
				return result, result.Error
			}

			result.Action = "recreated"
			result.Success = true
			result.Message = fmt.Sprintf(ConstStreamRecreatedFileWithUpdatedHashOldStrNewStr, hash, calculatedHash)
			if options.Logger != nil {
				options.Logger.Info("cas_recovered")
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
				options.Logger.Info("cas_recovered")
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
	storageForKind any,
) ([]*CASRecoveryResult, error) {
	if options == nil {
		options = &CASRecoveryOptions{
			RemoveFromIndexIfMissing: true,
			RecreateFromIDBased:      true,
			Logger:                   nil, // No logger by default
		}
	}

	kindDir, err := resolveKindDir(projectRoot, kind)
	if err != nil {
		return nil, err
	}

	var cas *filecas.ContentAddressableStorage
	var allMappings map[string]string
	if fileStorage, ok := storageForKind.(StorageFacade); ok && fileStorage != nil {
		cas, err = fileStorage.GetContentAddressableStorage(kind)
		if err == nil && cas != nil {
			allMappings, err = cas.GetAllMappings()
		}
	}
	if allMappings == nil || err != nil {
		cas = filecas.NewContentAddressableStorage(kindDir, kind)
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
		if _, err := fileutil.Stat(hashFile); err != nil {
			// File missing - attempt recovery (pass shared CAS to avoid repeated index load)
			result, err := RecoverCASObject(ctx, projectRoot, objectID, kind, options, cas)
			if err != nil {
				// Log error but continue with other objects
				if options.Logger != nil {
					options.Logger.Warn("cas_recovery_failed")
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
	kindDir, err := resolveKindDir(projectRoot, kind)
	if err != nil {
		return nil, err
	}
	cas := filecas.NewContentAddressableStorage(kindDir, kind)

	// Get all mappings from index
	allMappings, err := cas.GetAllMappings()
	if err != nil {
		return nil, errfmt.Newf(ConstStreamFailedToGetAllMappings).Wrap(err)
	}

	var issues []*CASRecoveryResult

	// Check each object
	for objectID, hash := range allMappings {
		hashFile := filepath.Join(kindDir, hash+".yaml")
		if _, err := fileutil.Stat(hashFile); err != nil {
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

func calculateSHA256Hash(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}
