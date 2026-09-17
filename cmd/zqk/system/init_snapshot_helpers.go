package system

import (
	"github.com/lanceman/zqk/pkg/datacell"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"

	stdcontext "context"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/internal/cli"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"

	"github.com/lanceman/zqk/pkg/objects"
)

// validateSnapshotFile validates that snapshot file exists
func validateSnapshotFile(snapshotPath string) error {
	if _, err := fileutil.Stat(snapshotPath); fileutil.IsNotExist(err) {
		return errfmt.Errorf("snapshot file not found: %s", snapshotPath)
	}
	return nil
}

// loadCompressedSnapshot loads and expands a compressed snapshot
func loadCompressedSnapshot(snapshotPath string, logger logging.Logger) ([]map[string]any, error) {
	cs, err := storage.ReadCompressedSnapshot(snapshotPath)
	if err != nil {
		return nil, errfmt.Newf("failed to read compressed snapshot").Wrap(err)
	}

	expanded, err := cs.Expand()
	if err != nil {
		return nil, errfmt.Newf("failed to expand snapshot").Wrap(err)
	}

	logging.Fluent(logger).Info("Loaded compressed snapshot").
		File(snapshotPath).
		Int("check_results", len(expanded)).
		String("timestamp", cs.Header.Timestamp.Format("2006-01-02T15:04:05Z")).
		Log()

	return expanded, nil
}

// findOriginalProjectRootFromSnapshotPath finds original project root from snapshot path
func findOriginalProjectRootFromSnapshotPath(snapshotPath string) string {
	snapshotDir := filepath.Dir(snapshotPath)
	if strings.Contains(snapshotDir, "test-scenarios") {
		// Snapshot is in a test scenario, original project is the main project
		return filepath.Clean(filepath.Join(snapshotDir, "..", ".."))
	}
	// Snapshot is in main project, use current directory or snapshot directory
	if cwd, err := fileutil.Getwd(); err == nil {
		return cwd
	}
	return filepath.Dir(snapshotPath)
}

// verifyProjectRoot verifies that a path is a valid project root
func verifyProjectRoot(root string) bool {
	processDir := datacell.ProcessPrimaryDir(root)
	_, err := fileutil.Stat(processDir)
	return err == nil
}

// findOriginalProjectRoot finds the original project root for snapshot restoration
func findOriginalProjectRoot(snapshotPath string, logger logging.Logger) (string, error) {
	// First, try to find project root from snapshot path location
	originalProjectRoot := findOriginalProjectRootFromSnapshotPath(snapshotPath)

	// Verify this is a valid project root
	if verifyProjectRoot(originalProjectRoot) {
		return originalProjectRoot, nil
	}

	// Try to find project root by looking for project data directory or go.mod
	foundRoot := cli.ResolveProjectRoot(originalProjectRoot)
	if foundRoot != emptyValue && foundRoot != originalProjectRoot && verifyProjectRoot(foundRoot) {
		return foundRoot, nil
	}

	// Last resort: try to find from current working directory
	if cwd, err := fileutil.Getwd(); err == nil {
		if strings.Contains(cwd, "test-scenarios") {
			originalProjectRoot = filepath.Clean(filepath.Join(cwd, "..", ".."))
		} else {
			originalProjectRoot = cwd
		}
		if verifyProjectRoot(originalProjectRoot) {
			return originalProjectRoot, nil
		}
	}

	return "", errfmt.Errorf("cannot determine original project root - compressed snapshot restoration requires access to original project with %s directory. Tried: %s", paths.ProcessDir, originalProjectRoot)
}

// unsetTestRoot temporarily unsets ZQK_TEST_ROOT for original project access
func unsetTestRoot() func() {
	originalTestRoot := zqkenv.TestRoot().Get()
	if originalTestRoot != emptyValue {
		_ = zqkenv.TestRoot().Unset()
		return func() {
			zqkenv.OSEnvSetter(zqkenv.TestRoot().Name(), originalTestRoot)
		}
	}
	return func() {}
}

// readObjectFromOriginalProject reads an object from original project storage
func readObjectFromOriginalProject(ctx stdcontext.Context, storageProvider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, objID, filePath, originalProjectRoot string, logger logging.Logger) (map[string]any, error) {
	// Try to read object by ID first (more reliable)
	obj, err := storageProvider.Read(ctx, secCtx, objID)
	if err == nil {
		return obj, nil
	}

	// If read by ID fails, try reading from file path
	if filePath == emptyValue {
		logging.Fluent(logger).Debug("Skipping object - no file path and ID read failed").
			ObjectID(objID).
			WithError(err).
			Log()
		return nil, errfmt.Errorf("object not found: %s", objID)
	}

	// Make path absolute if relative
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(originalProjectRoot, filePath)
	}

	// Read file directly
	data, readErr := fileutil.ReadFile(filePath)
	if readErr != nil {
		logging.Fluent(logger).Debug("Failed to read object from file path").
			ObjectID(objID).
			Path(filePath).
			WithError(readErr).
			Log()
		return nil, readErr
	}

	var fileObj map[string]any
	if unmarshalErr := yaml.Unmarshal(data, &fileObj); unmarshalErr != nil {
		logging.Fluent(logger).Debug("Failed to parse object YAML").
			ObjectID(objID).
			Path(filePath).
			WithError(unmarshalErr).
			Log()
		return nil, unmarshalErr
	}

	return fileObj, nil
}

// ensureObjectFields ensures object has required fields
func ensureObjectFields(obj map[string]any, objID, objKind string) {
	if obj[objects.FieldKeyID] == nil {
		obj[objects.FieldKeyID] = objID
	}
	if obj[objects.FieldKeyKind] == nil {
		obj[objects.FieldKeyKind] = objKind
	}
}

// convertCompressedSnapshotToObjects converts compressed snapshot check results to objects
func convertCompressedSnapshotToObjects(expanded []map[string]any, originalProjectRoot string, logger logging.Logger) ([]map[string]any, error) {
	ctx := pkgctx.NewSystemContext()
	originalFactory, err := storage.NewStorageFactory(ctx, originalProjectRoot)
	if err != nil {
		return nil, errfmt.Newf("failed to create storage factory for original project").Wrap(err)
	}

	originalStorage := originalFactory.GetStorage()
	secCtx := pkgctx.NewSystemSecurityContext()

	snapshotObjects := make([]map[string]any, 0, len(expanded))
	for i, resultMap := range expanded {
		// Extract object ID and kind from check result
		objID, _ := resultMap["object_id"].(string)
		objKind, _ := resultMap[objects.FieldKeyObjectKind].(string)
		filePath, _ := resultMap[objects.FieldKeyFilePath].(string)

		if objID == emptyValue || objKind == emptyValue {
			logging.Fluent(logger).Debug("Skipping check result without ID or kind").
				Int("index", i).
				Log()
			continue
		}

		obj, err := readObjectFromOriginalProject(ctx, originalStorage, secCtx, objID, filePath, originalProjectRoot, logger)
		if err != nil {
			continue // Already logged
		}

		ensureObjectFields(obj, objID, objKind)
		snapshotObjects = append(snapshotObjects, obj)

		if len(snapshotObjects)%1000 == 0 {
			logging.Fluent(logger).Info("Loading objects from original project...").
				Loaded(len(snapshotObjects)).
				Total(len(expanded)).
				Log()
		}
	}

	logging.Fluent(logger).Info("Loaded objects from original project").
		Loaded(len(snapshotObjects)).
		TotalCheckResults(len(expanded)).
		Log()

	return snapshotObjects, nil
}

// convertJSONSnapshotToObjects converts JSON snapshot check results to objects
func convertJSONSnapshotToObjects(snapshot *CheckSnapshot, logger logging.Logger) ([]map[string]any, error) {
	originalProjectRoot := snapshot.Metadata.ProjectRoot
	if originalProjectRoot == emptyValue {
		return nil, errfmt.Errorf("snapshot metadata missing project_root - cannot restore objects")
	}

	ctx := pkgctx.NewSystemContext()
	originalFactory, err := storage.NewStorageFactory(ctx, originalProjectRoot)
	if err != nil {
		return nil, errfmt.Newf("failed to create storage factory for original project").Wrap(err)
	}

	originalStorage := originalFactory.GetStorage()
	secCtx := pkgctx.NewSystemSecurityContext()

	snapshotObjects := make([]map[string]any, 0, len(snapshot.Results))
	for _, result := range snapshot.Results {
		obj, err := readObjectFromOriginalProject(ctx, originalStorage, secCtx, result.ObjectID, result.FilePath, originalProjectRoot, logger)
		if err != nil {
			continue // Already logged
		}

		ensureObjectFields(obj, result.ObjectID, result.ObjectKind)
		snapshotObjects = append(snapshotObjects, obj)
	}

	logging.Fluent(logger).Info("Loaded objects from original project").
		Int("loaded", len(snapshotObjects)).
		Int("total_results", len(snapshot.Results)).
		Log()

	return snapshotObjects, nil
}

// handleWipeMode handles wipe mode for snapshot initialization
func handleWipeMode(projectRoot string, wipe, force bool, logger logging.Logger) error {
	if !wipe {
		return nil
	}

	processDir := datacell.ProcessPrimaryDir(projectRoot)
	if _, err := fileutil.Stat(processDir); err == nil {
		if !force {
			return errfmt.Errorf("wipe mode requires --force to confirm data deletion")
		}
		logging.Fluent(logger).Warn("Wiping existing "+paths.ProcessDir+" data").
			String("directory", processDir).
			Log()
		// TODO: Implement safe wipe (backup first?)
		// For now, just log warning
	}

	return nil
}

// setupSnapshotProjectDirectories sets up project directories for snapshot initialization
func setupSnapshotProjectDirectories(projectRoot string, force bool) error {
	projectDataDir := filepath.Join(projectRoot, paths.ProjectDataDir)
	processDir := datacell.ProcessPrimaryDir(projectRoot)

	if err := createProjectDataDir(projectDataDir, force); err != nil {
		return errfmt.Newf("failed to create project data directory").Wrap(err)
	}

	if err := createAllKindDirectories(processDir); err != nil {
		return errfmt.Newf("failed to create kind directories").Wrap(err)
	}

	return nil
}
