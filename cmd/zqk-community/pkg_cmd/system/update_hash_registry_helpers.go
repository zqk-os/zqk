package system

import (
	stdcontext "context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"gopkg.in/yaml.v3"
)

// HashRegistryUpdateContext groups state for hash registry updates
type HashRegistryUpdateContext struct {
	Cache         *ObjectIDCache
	ProjectRoot   string
	AffectedKinds []string
	KindToFiles   map[string][]string
}

// initializeHashRegistryUpdateContext sets up the hash registry update context
func initializeHashRegistryUpdateContext(cache *ObjectIDCache, projectRoot string, affectedKinds []string) *HashRegistryUpdateContext {
	return &HashRegistryUpdateContext{
		Cache:         cache,
		ProjectRoot:   projectRoot,
		AffectedKinds: affectedKinds,
		KindToFiles:   make(map[string][]string),
	}
}

// collectModifiedFiles collects files that need hash registry updates
func collectModifiedFiles(updateCtx *HashRegistryUpdateContext) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	systemCtx := pkgctx.NewSystemContext()
	ctx, cancel := stdcontext.WithTimeout(systemCtx, 5*time.Second)
	defer cancel()
	_ = concurrency.WithRLockTimeout(
		&updateCtx.Cache.mu,
		ctx,
		nil,
		logging.NewLockLoggerAdapter(logger),
		LockNameHashRegistryCollectModified,
		func() error {
			for kind, list := range updateCtx.Cache.byKind {
				for i := range list {
					entry := updateCtx.Cache.entryFromBucket(kind, &list[i])
					if !shouldProcessEntry(updateCtx, entry) {
						continue
					}
					if isFileModified(entry) {
						if updateCtx.KindToFiles[entry.Kind] == nil {
							updateCtx.KindToFiles[entry.Kind] = make([]string, 0)
						}
						updateCtx.KindToFiles[entry.Kind] = append(updateCtx.KindToFiles[entry.Kind], entry.FilePath)
					}
				}
			}
			return nil
		},
	)
}

// shouldProcessEntry determines if an entry should be processed
func shouldProcessEntry(updateCtx *HashRegistryUpdateContext, entry *ObjectIDCacheEntry) bool {
	if len(updateCtx.AffectedKinds) == 0 {
		return true
	}

	for _, kind := range updateCtx.AffectedKinds {
		if entry.Kind == kind {
			return true
		}
	}
	return false
}

// isFileModified checks if a file has been modified
func isFileModified(entry *ObjectIDCacheEntry) bool {
	info, err := os.Stat(entry.FilePath)
	if err != nil {
		return false // File doesn't exist, skip
	}

	timeDiff := info.ModTime().Sub(entry.MTime)
	return timeDiff < -time.Second || timeDiff > time.Second
}

// isCASFile checks if a file is a CAS file (hash-based filename)
func isCASFile(filename string) bool {
	return len(filename) == 69 && strings.HasSuffix(filename, ".yaml") && // 64 chars + ".yaml"
		isHexString(filename[:64])
}

// extractObjectIDFromYAMLFile extracts object ID from YAML file content
func extractObjectIDFromYAMLFile(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	var obj map[string]any
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return "", err
	}

	objectID, ok := obj[objects.FieldKeyID].(string)
	if !ok || objectID == emptyValue {
		return "", errfmt.Errorf("object ID not found in file")
	}

	return objectID, nil
}

// checkCASHashMismatch checks if a CAS file has a hash mismatch
// Returns true if content hash != filename hash, along with the content hash
func checkCASHashMismatch(filePath string) (bool, string, error) {
	filename := filepath.Base(filePath)
	if !isCASFile(filename) {
		return false, "", nil
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return false, "", err
	}

	actualHash := storage.CalculateSHA256Hash(content)
	filenameHash := filename[:64] // Remove .yaml extension

	return actualHash != filenameHash, actualHash, nil
}

// fixCASHashMismatch fixes a CAS hash mismatch by calling storage.Update()
// This will recalculate the hash, rename the file, and update the CAS index
func fixCASHashMismatch(projectRoot, filePath, objectID, kind string) error {
	systemCtx := pkgctx.NewSystemContext()
	storageFactory, err := storage.NewStorageFactory(systemCtx, projectRoot)
	if err != nil {
		return errfmt.Newf("failed to create storage factory").Wrap(err)
	}

	storageProvider := storageFactory.GetStorage()
	if storageProvider == nil {
		return errfmt.Errorf("storage provider is nil")
	}

	secCtx := &pkgctx.SecurityContext{
		AccountID:   "system",
		Roles:       []string{"admin"},
		Permissions: []string{"write:*"},
	}

	// Call Update with empty updates - this will trigger hash recalculation
	// The storage provider will read the current content, recalculate the hash,
	// rename the file if needed, and update the CAS index
	updateErr := storageProvider.Update(systemCtx, secCtx, objectID, map[string]any{})
	if updateErr != nil {
		return errfmt.Newf("storage provider Update() failed").Wrap(updateErr)
	}

	return nil
}

// updateHashRegistryForKind updates hash registry for a specific kind
// For CAS files, also checks for and fixes hash mismatches
func updateHashRegistryForKind(updateCtx *HashRegistryUpdateContext, kind string, filePaths []string) int {
	kindDir := getKindDirectory(updateCtx.ProjectRoot, kind)
	if kindDir == emptyValue {
		return 0 // Unknown kind, skip
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	updatedCount := 0
	hasNonCASFiles := false

	// Create registry once for non-CAS files
	// Note: Helper function may be called from various contexts
	// Use system context for utility functions
	systemCtx := pkgctx.NewSystemContext()
	registry := storage.NewHashRegistry(systemCtx, kind, kindDir)
	if err := registry.Load(); err != nil {
		// Registry doesn't exist yet, create it
	}

	for _, filePath := range filePaths {
		filename := filepath.Base(filePath)

		// Check if this is a CAS file
		if isCASFile(filename) {
			// For CAS files, check for hash mismatches and fix them
			hasMismatch, _, err := checkCASHashMismatch(filePath)
			if err != nil {
				logging.Fluent(logger).Debug("Failed to check CAS hash mismatch").
					File(filePath).
					WithError(err).
					Log()
				continue
			}

			if hasMismatch {
				// Extract object ID from file content
				objectID, err := extractObjectIDFromYAMLFile(filePath)
				if err != nil {
					logging.Fluent(logger).Debug("Failed to extract object ID from CAS file").
						File(filePath).
						WithError(err).
						Log()
					continue
				}

				// Fix hash mismatch using storage.Update()
				if err := fixCASHashMismatch(updateCtx.ProjectRoot, filePath, objectID, kind); err != nil {
					logging.Fluent(logger).Warn("Failed to fix CAS hash mismatch").
						File(filePath).
						String("object_id", objectID).
						WithError(err).
						Log()
					continue
				}

				updatedCount++
				logging.Fluent(logger).Debug("Fixed CAS hash mismatch").
					File(filePath).
					String("object_id", objectID).
					Log()
			}
			// CAS files don't use hash registries, so we skip the hash registry update
			continue
		}

		// For non-CAS files, update hash registry
		if updateFileHash(registry, filePath) {
			updatedCount++
			hasNonCASFiles = true
		}
	}

	// Save hash registry for non-CAS files (only if we updated any)
	if hasNonCASFiles {
		if err := registry.Save(); err != nil {
			logging.Fluent(logger).Warn("Failed to save hash registry").
				Kind(kind).
				WithError(err).
				Log()
		}
	}

	return updatedCount
}

// updateFileHash updates hash for a single file
func updateFileHash(registry storage.HashRegistryProvider, filePath string) bool {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return false // Skip if can't read
	}

	hash := sha256.Sum256(content)
	hashStr := hex.EncodeToString(hash[:])

	filename := filepath.Base(filePath)
	registry.SetHash(filename, hashStr)
	return true
}
