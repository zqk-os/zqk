package storage

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/appledouble"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

// HashMigration migrates individual .hash files to hash index files by kind
type HashMigration struct {
	rootPath string
	verbose  bool
	logger   *logging.EventLogger
}

// NewHashMigration creates a new hash migration utility
func NewHashMigration(rootPath string) *HashMigration {
	return &HashMigration{
		rootPath: rootPath,
		logger:   logging.NewEventLogger(pkgctx.NewSystemContext()),
	}
}

// SetVerbose enables verbose output
func (hm *HashMigration) SetVerbose(verbose bool) {
	hm.verbose = verbose
}

// SetLogger sets a custom logger for the migration
func (hm *HashMigration) SetLogger(logger *logging.EventLogger) {
	hm.logger = logger
}

// MigrationResult contains the results of a hash migration
type MigrationResult struct {
	KindsProcessed []string
	HashesMigrated int
	HashesSkipped  int
	Errors         []error
	OldHashFiles   []string // List of old .hash files that were migrated
}

// Migrate migrates all individual .hash files to hash index files
// Returns a MigrationResult with statistics about the migration
func (hm *HashMigration) Migrate(dryRun bool) (*MigrationResult, error) {
	result := &MigrationResult{
		KindsProcessed: []string{},
		HashesMigrated: 0,
		HashesSkipped:  0,
		Errors:         []error{},
		OldHashFiles:   []string{},
	}

	// Map to track hashes by kind
	hashesByKind := make(map[string]map[string]string) // kind -> filename -> hash

	// Scan for all .hash files
	hashFiles, err := hm.findHashFiles()
	if err != nil {
		return nil, errfmt.Newf(ConstMiscFailedToFindHashFiles).Wrap(err)
	}

	if hm.verbose {
		StorageLog(hm.logger.Logger()).Info(LogEventStorageHashMigrationFoundFilesInfo).
			Int("count", len(hashFiles)).
			Log()
	}

	// Process each hash file
	for _, hashFile := range hashFiles {
		// Determine object kind from directory
		kind, filename, err := hm.parseHashFile(hashFile)
		if err != nil {
			result.Errors = append(result.Errors, errfmt.Errorf(ConstMiscFailedToParseSW, hashFile, err))
			continue
		}

		// Read hash value from .hash file
		hash, err := os.ReadFile(hashFile)
		if err != nil {
			result.Errors = append(result.Errors, errfmt.Errorf(ConstMiscFailedToReadSW, hashFile, err))
			continue
		}

		hashStr := strings.TrimSpace(string(hash))
		if hashStr == emptyValue {
			result.Errors = append(result.Errors, errfmt.Errorf(ConstMiscEmptyHashInS, hashFile))
			continue
		}

		// Initialize kind map if needed
		if hashesByKind[kind] == nil {
			hashesByKind[kind] = make(map[string]string)
		}

		// Check if hash already exists in index (if it does, skip)
		kindDir := hm.getKindDirectory(kind)
		registry := NewHashRegistry(pkgctx.NewSystemContext(), kind, kindDir)
		if err := registry.Load(); err == nil {
			if existingHash := registry.GetHash(filename); existingHash != emptyValue {
				if existingHash == hashStr {
					// Hash already exists in index, skip
					result.HashesSkipped++
					if hm.verbose {
						StorageLog(hm.logger.Logger()).Debug(LogEventStorageHashMigrationSkippingIndexedDebug).
							String("file", hashFile).
							Log()
					}
					continue
				}
				// Hash mismatch - this is a conflict
				result.Errors = append(result.Errors, errfmt.Errorf(ConstMiscHashMismatchForSIndexHasSHashFileHasS, filename, existingHash, hashStr))
				continue
			}
		}

		// Add to migration map
		hashesByKind[kind][filename] = hashStr
		result.OldHashFiles = append(result.OldHashFiles, hashFile)
	}

	// Write hash indexes for each kind
	for kind, hashes := range hashesByKind {
		if len(hashes) == 0 {
			continue
		}

		kindDir := hm.getKindDirectory(kind)
		registry := NewHashRegistry(pkgctx.NewSystemContext(), kind, kindDir)

		// Load existing registry (if any)
		if err := registry.Load(); err != nil && !os.IsNotExist(err) {
			result.Errors = append(result.Errors, errfmt.Errorf(ConstMiscFailedToLoadExistingRegistryForSW, kind, err))
			continue
		}

		// Add migrated hashes
		for filename, hash := range hashes {
			registry.SetHash(filename, hash)
			result.HashesMigrated++
		}

		// Save registry
		if !dryRun {
			if err := registry.Save(); err != nil {
				result.Errors = append(result.Errors, errfmt.Errorf(ConstMiscFailedToSaveRegistryForSW, kind, err))
				continue
			}
			if hm.verbose {
				StorageLog(hm.logger.Logger()).Info(LogEventStorageHashMigrationRegistryUpdatedInfo).
					Kind(kind).
					String("file", registry.filePath).
					Int("hash_count", len(hashes)).
					Log()
			}
		} else if hm.verbose {
			StorageLog(hm.logger.Logger()).Info(LogEventStorageHashMigrationDryRunWouldUpdateInfo).
				Kind(kind).
				String("file", registry.filePath).
				Int("hash_count", len(hashes)).
				Log()
		}

		result.KindsProcessed = append(result.KindsProcessed, kind)
	}

	// Remove old .hash files if not dry run
	if !dryRun && len(result.OldHashFiles) > 0 {
		for _, oldHashFile := range result.OldHashFiles {
			if err := os.Remove(oldHashFile); err != nil {
				result.Errors = append(result.Errors, errfmt.Errorf(ConstMiscFailedToRemoveSW, oldHashFile, err))
			} else if hm.verbose {
				StorageLog(hm.logger.Logger()).Info(LogEventStorageHashMigrationRemovedOldFileInfo).
					String("file", oldHashFile).
					Log()
			}
		}
	}

	return result, nil
}

// findHashFiles finds all .hash files in the root path
func (hm *HashMigration) findHashFiles() ([]string, error) {
	var hashFiles []string

	err := filepath.Walk(hm.rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		if appledouble.SkipPathInTreeWalk(path) {
			return nil
		}

		// Check if file ends with .hash
		if strings.HasSuffix(path, ".hash") {
			hashFiles = append(hashFiles, path)
		}

		return nil
	})

	return hashFiles, err
}

// parseHashFile extracts the object kind and filename from a hash file path
// Example: "docs/architecture/backlog/ITEM-001.yaml.hash" -> kind="backlog_item", filename="ITEM-001.yaml"
func (hm *HashMigration) parseHashFile(hashFilePath string) (kind, filename string, err error) {
	// Remove .hash extension
	objectPath := strings.TrimSuffix(hashFilePath, ".hash")

	// Get directory and filename
	dir := filepath.Dir(objectPath)
	filename = filepath.Base(objectPath)

	// Determine kind from directory name
	// Map directory names to object kinds
	dirBase := filepath.Base(dir)
	kind = hm.directoryToKind(dirBase)

	if kind == emptyValue {
		return "", "", errfmt.Errorf(ConstMiscUnknownObjectKindForDirectoryS, dirBase)
	}

	return kind, filename, nil
}

// directoryToKind maps directory names to object kinds
func (hm *HashMigration) directoryToKind(dirName string) string {
	return objects.GetKindFromDirectory(dirName)
}

// getKindDirectory returns the directory path for a given object kind
func (hm *HashMigration) getKindDirectory(kind string) string {
	dirName := objects.GetDirectoryFromKind(kind)
	if dirName == emptyValue {
		return hm.rootPath
	}

	return filepath.Join(hm.rootPath, dirName)
}

// VerifyHash verifies that a file's hash matches the expected hash
func VerifyHash(filePath, expectedHash string) (bool, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return false, errfmt.Newf(ConstMiscFailedToReadFile).Wrap(err)
	}

	// Use shared hash calculation function to ensure consistency
	actualHash := CalculateSHA256Hash(data)

	return actualHash == expectedHash, nil
}
