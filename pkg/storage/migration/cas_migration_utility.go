package migration

import (
	"context"
	"path/filepath"

	"github.com/lanceman/zqk/pkg/storage"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
)

// MigrateObjectToCAS migrates a single object from ID-based format to CAS
// This reads the existing ID-based file, creates it in CAS, and optionally removes the old file
// oldFilePath is optional - if provided, uses this exact path for removal (handles bucketed storage)
func (m *CASMigrationUtility) MigrateObjectToCAS(ctx context.Context, secCtx *pkgctx.SecurityContext, objectID string, removeOldFile bool, oldFilePath ...string) error {
	// Infer kind from ID
	if err := m.facade.GetIDValidator().LoadPatterns(); err != nil {
		return errfmt.Newf(storage.ConstStreamFailedToLoadIdPatterns).Wrap(err)
	}
	kind := m.facade.GetIDValidator().InferKindFromID(objectID)
	if kind == "" {
		return errfmt.Errorf(storage.ConstStreamCouldNotInferKindFromIdStr, objectID)
	}

	// Check if this kind uses CAS
	if !m.facade.UsesContentAddressableStorage(kind) {
		return errfmt.Errorf(storage.ConstStreamKindStrDoesNotUseContentAddressableStorage, kind)
	}

	// Read existing object
	obj, err := m.facade.Read(ctx, secCtx, objectID)
	if err != nil {
		return errfmt.Newf(storage.ConstStreamFailedToReadObject).Wrap(err)
	}

	// CRITICAL: Validate object before migration to ensure we don't migrate invalid objects
	// This ensures migration preserves validation state and doesn't create invalid objects
	if err := m.facade.ValidateObject(ctx, obj, kind, ""); err != nil {
		return errfmt.Newf(storage.ConstStreamObjectValidationFailedBeforeMigrationSkippingTo).Wrap(err)
	}

	// Check if object already exists in CAS
	cas, err := m.facade.GetContentAddressableStorage(kind)
	if err != nil {
		return errfmt.Newf(storage.ErrMsgGetCAS).Wrap(err)
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
			return errfmt.Newf(storage.ConstStreamFailedToMarshalObject).Wrap(err)
		}

		// Create in CAS (validation already passed above)
		if err := cas.Create(objectID, data); err != nil {
			return errfmt.Newf(storage.ConstStreamFailedToCreateObjectInCas).Wrap(err)
		}
	}

	// Remove old ID-based file if requested (even if already in CAS)
	if removeOldFile {
		var pathToRemove string
		if len(oldFilePath) > 0 && oldFilePath[0] != "" {
			// Use provided path (handles bucketed storage)
			pathToRemove = oldFilePath[0]
			// Ensure path is absolute (in case it's relative)
			if !filepath.IsAbs(pathToRemove) {
				// If relative, make it relative to project root
				pathToRemove = filepath.Join(m.facade.GetProjectRoot(), pathToRemove)
			}
		} else {
			// Fall back to reconstructing path (for flat storage)
			var err error
			pathToRemove, err = m.facade.GetObjectFilePath(objectID, kind)
			if err != nil {
				// Can't determine path - skip removal
				return nil
			}
		}

		if _, statErr := fileutil.Stat(pathToRemove); statErr == nil {
			// Old file exists - remove it
			if err := fileutil.Remove(pathToRemove); err != nil {
				// Log but don't fail - file removal is best effort
				return errfmt.Newf(storage.ConstStreamFailedToRemoveOldFileObjectMigratedToCas).Wrap(err)
			}
		} else if !fileutil.IsNotExist(statErr) {
			// Log if there's an error other than file not existing
			// (file not existing is fine - it might have been removed already)
		}
	}

	return nil
}

// MigrateKindToCAS migrates all objects of a specific kind from ID-based format to CAS
func (m *CASMigrationUtility) MigrateKindToCAS(ctx context.Context, secCtx *pkgctx.SecurityContext, kind string, removeOldFiles bool) (int, []error) {
	// Check if this kind uses CAS
	if !m.facade.UsesContentAddressableStorage(kind) {
		return 0, []error{errfmt.Errorf(storage.ConstStreamKindStrDoesNotUseContentAddressableStorage, kind)}
	}

	// Get directory for this kind
	dirName := objects.GetDirectoryFromKind(kind)
	if dirName == "" {
		return 0, []error{errfmt.Errorf(storage.ConstStreamUnknownObjectKindStr, kind)}
	}

	kindDir := filepath.Join(m.facade.GetProcessDir(), dirName)

	// Scan for ID-based files (handles both flat and bucketed storage)
	// Returns map of object ID to file path
	idToPath := m.facade.ScanIDBasedFilesRecursive(kindDir, kind)

	var migrated int
	var errors []error

	// Migrate each object
	for objectID, filePath := range idToPath {
		if err := m.MigrateObjectToCAS(ctx, secCtx, objectID, removeOldFiles, filePath); err != nil {
			errors = append(errors, errfmt.Errorf(storage.ConstStreamFailedToMigrateStrErr, objectID, err))
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
func (m *CASMigrationUtility) MigrateAllToCAS(ctx context.Context, secCtx *pkgctx.SecurityContext, removeOldFiles bool) (map[string]int, map[string][]error) {
	// Discover all object kinds
	kinds := m.facade.DiscoverObjectKinds()

	migratedByKind := make(map[string]int)
	errorsByKind := make(map[string][]error)

	// Migrate each kind that uses CAS
	for _, kind := range kinds {
		if !m.facade.UsesContentAddressableStorage(kind) {
			continue
		}

		migrated, errors := m.MigrateKindToCAS(ctx, secCtx, kind, removeOldFiles)
		migratedByKind[kind] = migrated
		if len(errors) > 0 {
			errorsByKind[kind] = errors
		}
	}

	return migratedByKind, errorsByKind
}

// discoverObjectKinds discovers all object kinds by scanning the process directory

// scanIDBasedFilesRecursive scans for ID-based files recursively,
// handling both flat storage and bucketed storage (date-based subdirectories)
// Returns a map of object ID to file path
