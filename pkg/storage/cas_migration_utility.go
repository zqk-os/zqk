package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"gopkg.in/yaml.v3"
)

// MigrateObjectToCAS migrates a single object from ID-based format to CAS
// This reads the existing ID-based file, creates it in CAS, and optionally removes the old file
// oldFilePath is optional - if provided, uses this exact path for removal (handles bucketed storage)
func (f *FileObjectStorage) MigrateObjectToCAS(ctx context.Context, secCtx *pkgctx.SecurityContext, objectID string, removeOldFile bool, oldFilePath ...string) error {
	// Infer kind from ID
	if err := f.idValidator.LoadPatterns(); err != nil {
		return errfmt.Newf(ConstStreamFailedToLoadIdPatterns).Wrap(err)
	}
	kind := f.idValidator.InferKindFromID(objectID)
	if kind == emptyValue {
		return errfmt.Errorf(ConstStreamCouldNotInferKindFromIdStr, objectID)
	}

	// Check if this kind uses CAS
	if !f.usesContentAddressableStorage(kind) {
		return errfmt.Errorf(ConstStreamKindStrDoesNotUseContentAddressableStorage, kind)
	}

	// Read existing object
	obj, err := f.Read(ctx, secCtx, objectID)
	if err != nil {
		return errfmt.Newf(ConstStreamFailedToReadObject).Wrap(err)
	}

	// CRITICAL: Validate object before migration to ensure we don't migrate invalid objects
	// This ensures migration preserves validation state and doesn't create invalid objects
	if err := f.validateObject(ctx, obj, kind, ""); err != nil {
		return errfmt.Newf(ConstStreamObjectValidationFailedBeforeMigrationSkippingTo).Wrap(err)
	}

	// Check if object already exists in CAS
	cas, err := f.getContentAddressableStorage(kind)
	if err != nil {
		return errfmt.Newf(ErrMsgGetCAS).Wrap(err)
	}

	_, err = cas.GetHashForID(objectID)
	alreadyInCAS := err == nil

	if alreadyInCAS && !removeOldFile {
		// Object already in CAS and we're not removing old files - skip
		return nil
	}

	if !alreadyInCAS {
		// Marshal object to YAML
		data, err := yaml.Marshal(obj)
		if err != nil {
			return errfmt.Newf(ConstStreamFailedToMarshalObject).Wrap(err)
		}

		// Create in CAS (validation already passed above)
		if err := cas.Create(objectID, data); err != nil {
			return errfmt.Newf(ConstStreamFailedToCreateObjectInCas).Wrap(err)
		}
	}

	// Remove old ID-based file if requested (even if already in CAS)
	if removeOldFile {
		var pathToRemove string
		if len(oldFilePath) > 0 && oldFilePath[0] != emptyValue {
			// Use provided path (handles bucketed storage)
			pathToRemove = oldFilePath[0]
			// Ensure path is absolute (in case it's relative)
			if !filepath.IsAbs(pathToRemove) {
				// If relative, make it relative to project root
				pathToRemove = filepath.Join(f.projectRoot, pathToRemove)
			}
		} else {
			// Fall back to reconstructing path (for flat storage)
			var err error
			pathToRemove, err = f.getObjectFilePath(objectID, kind)
			if err != nil {
				// Can't determine path - skip removal
				return nil
			}
		}

		if _, statErr := os.Stat(pathToRemove); statErr == nil {
			// Old file exists - remove it
			if err := os.Remove(pathToRemove); err != nil {
				// Log but don't fail - file removal is best effort
				return errfmt.Newf(ConstStreamFailedToRemoveOldFileObjectMigratedToCas).Wrap(err)
			}
		} else if !os.IsNotExist(statErr) {
			// Log if there's an error other than file not existing
			// (file not existing is fine - it might have been removed already)
		}
	}

	return nil
}

// MigrateKindToCAS migrates all objects of a specific kind from ID-based format to CAS
func (f *FileObjectStorage) MigrateKindToCAS(ctx context.Context, secCtx *pkgctx.SecurityContext, kind string, removeOldFiles bool) (int, []error) {
	// Check if this kind uses CAS
	if !f.usesContentAddressableStorage(kind) {
		return 0, []error{errfmt.Errorf(ConstStreamKindStrDoesNotUseContentAddressableStorage, kind)}
	}

	// Get directory for this kind
	dirName := objects.GetDirectoryFromKind(kind)
	if dirName == emptyValue {
		return 0, []error{errfmt.Errorf(ConstStreamUnknownObjectKindStr, kind)}
	}

	kindDir := filepath.Join(f.processDir, dirName)

	// Scan for ID-based files (handles both flat and bucketed storage)
	// Returns map of object ID to file path
	idToPath := f.scanIDBasedFilesRecursive(kindDir, kind)

	var migrated int
	var errors []error

	// Migrate each object
	for objectID, filePath := range idToPath {
		if err := f.MigrateObjectToCAS(ctx, secCtx, objectID, removeOldFiles, filePath); err != nil {
			errors = append(errors, errfmt.Errorf(ConstStreamFailedToMigrateStrErr, objectID, err))
			continue
		}
		migrated++
	}

	return migrated, errors
}

// MigrateAllToCAS migrates all objects in the project to CAS (for kinds that use CAS)
// This is useful for test scenarios or full migration
//
//nolint:gocritic // Multiple return values intentional for summary/error map
func (f *FileObjectStorage) MigrateAllToCAS(ctx context.Context, secCtx *pkgctx.SecurityContext, removeOldFiles bool) (map[string]int, map[string][]error) {
	// Discover all object kinds
	kinds := f.discoverObjectKinds()

	migratedByKind := make(map[string]int)
	errorsByKind := make(map[string][]error)

	// Migrate each kind that uses CAS
	for _, kind := range kinds {
		if !f.usesContentAddressableStorage(kind) {
			continue
		}

		migrated, errors := f.MigrateKindToCAS(ctx, secCtx, kind, removeOldFiles)
		migratedByKind[kind] = migrated
		if len(errors) > 0 {
			errorsByKind[kind] = errors
		}
	}

	return migratedByKind, errorsByKind
}

// discoverObjectKinds discovers all object kinds by scanning the process directory
func (f *FileObjectStorage) discoverObjectKinds() []string {
	var kinds []string
	seen := make(map[string]bool)

	// Scan process directory for subdirectories
	entries, err := os.ReadDir(f.processDir)
	if err != nil {
		return kinds
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Try to infer kind from directory name
		dirName := entry.Name()
		kind := objects.GetKindFromDirectory(dirName)
		if kind != emptyValue && !seen[kind] {
			kinds = append(kinds, kind)
			seen[kind] = true
		}
	}

	return kinds
}

// scanIDBasedFilesRecursive scans for ID-based files recursively,
// handling both flat storage and bucketed storage (date-based subdirectories)
// Returns a map of object ID to file path
func (f *FileObjectStorage) scanIDBasedFilesRecursive(kindDir, kind string) map[string]string {
	idToPath := make(map[string]string)

	// Check if this kind uses bucketed storage
	if f.usesBucketedStorage(kind, kindDir) {
		// For bucketed storage, scan all date subdirectories
		entries, err := os.ReadDir(kindDir)
		if err != nil {
			if os.IsNotExist(err) {
				return idToPath
			}
			return idToPath
		}

		// Pattern for date directories (YYYY-MM or YYYY-MM-DD)
		datePattern := regexp.MustCompile(`^\d{4}-\d{2}(-\d{2})?$`)

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			// Check if it's a date directory
			if datePattern.MatchString(entry.Name()) {
				dateDir := filepath.Join(kindDir, entry.Name())
				// Recursively scan this date directory
				dateIDToPath := f.scanIDBasedFilesWithPaths(dateDir, kind)
				for id, path := range dateIDToPath {
					idToPath[id] = path
				}
			}
		}
	} else {
		// For flat storage, scan the kind directory directly
		flatIDToPath := f.scanIDBasedFilesWithPaths(kindDir, kind)
		for id, path := range flatIDToPath {
			idToPath[id] = path
		}
	}

	return idToPath
}

// scanIDBasedFilesWithPaths scans for ID-based files and returns a map of ID to file path
func (f *FileObjectStorage) scanIDBasedFilesWithPaths(kindDir, kind string) map[string]string {
	idToPath := make(map[string]string)
	entries, err := os.ReadDir(kindDir)
	if err != nil {
		return idToPath
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}

		// Skip hash-based files (64-char hex) - uses package-level compiled regex
		if casHashFilenameRe.MatchString(entry.Name()) {
			continue
		}

		// Skip index files
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		// Get full file path
		filePath := filepath.Join(kindDir, entry.Name())

		// Extract ID from filename
		filenameID := strings.TrimSuffix(entry.Name(), ".yaml")
		filenameID = strings.TrimSuffix(filenameID, ".yml")

		// For accounts, handle account-username format
		if kind == objects.KindAccount && strings.HasPrefix(filenameID, "account-") {
			username := strings.TrimPrefix(filenameID, "account-")
			filenameID = fmt.Sprintf("account:%s", username)
		}

		// Validate that this looks like an ID (not a hash)
		// IDs typically have a prefix and numbers (e.g., ITEM-001, TAM-100)
		if filenameID != emptyValue {
			idToPath[filenameID] = filePath
		}
	}

	return idToPath
}
