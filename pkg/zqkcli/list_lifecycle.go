package internal

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/appledouble"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// listLifecycleDefinitionsForAll returns lifecycle definitions as QueryResult (for use in listAllInternalObjects)
func listLifecycleDefinitionsForAll(cmd *cobra.Command, proc *cli.Processor, storageProvider storage.ObjectStorageProvider, projectRoot string, builtInOnly, internalOnly, allObjects bool) (*storage.QueryResult, error) {
	_ = cmd // Reserved for future use (e.g., flag parsing)
	// Check if this is a graph backend by trying to query for lifecycle objects
	secCtx := proc.SecurityContext()
	storageCtx := proc.StorageContext()

	// Try to query lifecycle objects from storage
	filter := storage.ListFilter{
		Kind:    internalKindLifecycle,
		Filters: make(map[string]any),
	}

	result, err := storageProvider.List(proc.OperationContext(), secCtx, storageCtx, filter)
	if err == nil && len(result.Objects) > 0 {
		// Graph backend has lifecycle objects stored as nodes
		// Apply filters
		filtered := make([]map[string]any, 0)
		for _, obj := range result.Objects {
			shouldInclude := false
			if allObjects {
				shouldInclude = true
			} else if builtInOnly {
				shouldInclude = storage.IsBuiltIn(obj)
			} else if internalOnly {
				shouldInclude = isInternalObject(obj)
			} else {
				shouldInclude = true
			}
			if shouldInclude {
				filtered = append(filtered, obj)
			}
		}
		result.Objects = filtered
		result.Meta["total_count"] = len(filtered)
		return result, nil
	}

	// Fall back to file-based scanning
	lifecyclesDir := filepath.Join(projectRoot, paths.ProcessInternalLifecyclesDir)
	if _, err := fileutil.Stat(lifecyclesDir); err != nil {
		if fileutil.IsNotExist(err) {
			return &storage.QueryResult{
				Objects: []map[string]any{},
				Meta:    map[string]any{"total_count": 0},
			}, nil
		}
		return nil, errfmt.Newf("failed to access lifecycles directory").Wrap(err)
	}

	var lifecycleObjects []map[string]any
	err = filepath.Walk(lifecyclesDir, func(path string, info fileutil.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if appledouble.SkipPathInTreeWalk(path) {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".yaml") && !strings.HasSuffix(info.Name(), ".yml") {
			return nil
		}

		lifecycleDef, loadErr := loadYAMLDoc(path)
		if loadErr != nil {
			return nil
		}

		objectType, _ := lifecycleDef[objects.FieldKeyObjectType].(string)
		if objectType == emptyValue {
			return nil
		}

		id := strings.TrimSuffix(info.Name(), filepath.Ext(info.Name()))
		isBuiltIn := strings.Contains(path, "/built-in/")

		obj := map[string]any{
			objects.FieldKeyID:         id,
			objects.FieldKeyKind:       internalKindLifecycle,
			objects.FieldKeyTitle:      fmt.Sprintf("%s Lifecycle", objectType),
			objects.FieldKeyObjectType: objectType,
			objects.FieldKeyFilePath:   path,
		}

		if schemaVersion, ok := lifecycleDef[objects.FieldKeySchemaVersion].(string); ok {
			obj[objects.FieldKeySchemaVersion] = schemaVersion
		}

		if isBuiltIn {
			obj[internalSourceType] = internalSourceBuiltIn
		} else {
			obj[internalSourceType] = internalSourceInternal
		}

		shouldInclude := false
		if allObjects {
			shouldInclude = true
		} else if builtInOnly {
			shouldInclude = isBuiltIn
		} else {
			// internalOnly or default: show all
			shouldInclude = true
		}

		if shouldInclude {
			lifecycleObjects = append(lifecycleObjects, obj)
		}

		return nil
	})

	if err != nil {
		return nil, errfmt.Newf("failed to scan lifecycle directory").Wrap(err)
	}

	return &storage.QueryResult{
		Objects: lifecycleObjects,
		Meta:    map[string]any{"total_count": len(lifecycleObjects)},
	}, nil
}

// listLifecycleDefinitions lists lifecycle definitions from file or graph backend
func listLifecycleDefinitions(cmd *cobra.Command, proc *cli.Processor, storageProvider storage.ObjectStorageProvider, projectRoot string, builtInOnly, internalOnly, allObjects bool, groupBy string, idsOnly bool) error {
	// Check if this is a graph backend by trying to query for lifecycle objects
	// Graph backend would have lifecycle objects stored as nodes
	secCtx := proc.SecurityContext()
	storageCtx := proc.StorageContext()

	// Try to query lifecycle objects from storage
	filter := storage.ListFilter{
		Kind:    internalKindLifecycle,
		Filters: make(map[string]any),
	}

	var result *storage.QueryResult
	var err error
	result, err = storageProvider.List(proc.OperationContext(), secCtx, storageCtx, filter)
	if err == nil && len(result.Objects) > 0 {
		// Graph backend has lifecycle objects stored as nodes
		// Apply filters
		filtered := make([]map[string]any, 0)
		for _, obj := range result.Objects {
			shouldInclude := false
			if allObjects {
				shouldInclude = true
			} else if builtInOnly {
				shouldInclude = storage.IsBuiltIn(obj)
			} else if internalOnly {
				shouldInclude = isInternalObject(obj)
			} else {
				// Default: show all lifecycle definitions (they're all internal)
				shouldInclude = true
			}
			if shouldInclude {
				filtered = append(filtered, obj)
			}
		}
		result.Objects = filtered
		result.Meta["total_count"] = len(filtered)

		// Apply grouping if requested
		if groupBy != emptyValue {
			result = groupResults(result, groupBy)
		}

		format := proc.Format()
		outputList(cmd, result, format, internalKindLifecycle, idsOnly)
		return nil
	}

	// Fall back to file-based scanning (for file backend or if graph has no lifecycle objects)
	lifecyclesDir := filepath.Join(projectRoot, paths.ProcessInternalLifecyclesDir)
	if _, err := fileutil.Stat(lifecyclesDir); err != nil {
		if fileutil.IsNotExist(err) {
			// No lifecycles directory - return empty result
			result = &storage.QueryResult{
				Objects: []map[string]any{},
				Meta:    map[string]any{"total_count": 0},
			}
			// Apply grouping if requested
			if groupBy != emptyValue {
				result = groupResults(result, groupBy)
			}
			format := proc.Format()
			outputList(cmd, result, format, internalKindLifecycle, idsOnly)
			return nil
		}
		return errfmt.Newf("failed to access lifecycles directory").Wrap(err)
	}

	// Scan for lifecycle YAML files
	var lifecycleObjects []map[string]any
	err = filepath.Walk(lifecyclesDir, func(path string, info fileutil.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil // Skip errors
		}
		if info.IsDir() {
			return nil // Continue into subdirectories
		}
		if appledouble.SkipPathInTreeWalk(path) {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".yaml") && !strings.HasSuffix(info.Name(), ".yml") {
			return nil
		}

		lifecycleDef, loadErr := loadYAMLDoc(path)
		if loadErr != nil {
			profile := proc.Context().Profile
			if profile == emptyValue {
				profile = string(pkgctx.ProfileSystem)
			}
			event := "parse_lifecycle_file"
			if fileutil.IsNotExist(loadErr) {
				event = "read_lifecycle_file"
			}
			emitInternalListErrorViaCoordinator(
				proc.OperationContext(),
				projectRoot,
				storageProvider,
				event,
				fmt.Sprintf("Failed to load lifecycle file: %s", path),
				loadErr,
				profile,
			)
			return nil
		}

		// Extract object_type (which is the kind this lifecycle applies to)
		objectType, _ := lifecycleDef[objects.FieldKeyObjectType].(string)
		if objectType == emptyValue {
			return nil // Skip files without object_type
		}

		// Create an object-like structure for display
		// Use filename as ID (without extension)
		id := strings.TrimSuffix(info.Name(), filepath.Ext(info.Name()))

		// Extract title from metadata or use object_type
		title := fmt.Sprintf("%s Lifecycle", objectType)
		if metadata, ok := lifecycleDef[objects.FieldKeyMetadata].(map[string]any); ok {
			if desc, ok := metadata[objects.FieldKeyDescription].(string); ok && desc != emptyValue {
				// Use first line of description as title
				lines := strings.Split(desc, "\n")
				if len(lines) > 0 && strings.TrimSpace(lines[0]) != emptyValue {
					trimmedTitle := strings.TrimSpace(lines[0])
					if len(trimmedTitle) > 80 {
						title = trimmedTitle[:77] + "..."
					} else {
						title = trimmedTitle
					}
				}
			}
		}

		// Build object representation
		obj := map[string]any{
			objects.FieldKeyID:         id,
			objects.FieldKeyKind:       internalKindLifecycle,
			objects.FieldKeyTitle:      title,
			objects.FieldKeyObjectType: objectType,
			objects.FieldKeyFilePath:   path,
		}

		if schemaVersion, ok := lifecycleDef[objects.FieldKeySchemaVersion].(string); ok {
			obj[objects.FieldKeySchemaVersion] = schemaVersion
		}

		// Add metadata if present
		if metadata, ok := lifecycleDef[objects.FieldKeyMetadata].(map[string]any); ok {
			obj[objects.FieldKeyMetadata] = metadata
		}

		// Check if this is a built-in lifecycle (in built-in subdirectory)
		isBuiltIn := strings.Contains(path, "/built-in/")
		if isBuiltIn {
			obj[internalSourceType] = internalSourceBuiltIn
		} else {
			obj[internalSourceType] = internalSourceInternal
		}

		// Apply filters
		shouldInclude := false
		if allObjects {
			shouldInclude = true
		} else if builtInOnly {
			shouldInclude = isBuiltIn
		} else {
			// internalOnly or default: show all lifecycle definitions (they're all internal)
			shouldInclude = true
		}

		if shouldInclude {
			lifecycleObjects = append(lifecycleObjects, obj)
		}

		return nil
	})

	if err != nil {
		return errfmt.Newf("failed to scan lifecycle directory").Wrap(err)
	}

	// Create result
	result = &storage.QueryResult{
		Objects: lifecycleObjects,
		Meta:    map[string]any{"total_count": len(lifecycleObjects)},
	}

	// Apply grouping if requested
	if groupBy != emptyValue {
		result = groupResults(result, groupBy)
	}

	// Output results
	format := proc.Format()
	outputList(cmd, result, format, internalKindLifecycle, idsOnly)

	return nil
}
