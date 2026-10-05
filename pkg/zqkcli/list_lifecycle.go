package internal

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/appledouble"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

func shouldIncludeLifecycle(isBuiltIn, isInternal, builtInOnly, internalOnly, allObjects bool) bool {
	if allObjects {
		return true
	}
	if builtInOnly {
		return isBuiltIn
	}
	if internalOnly {
		return isInternal
	}
	return true
}

func filterStorageLifecycles(objects []map[string]any, builtInOnly, internalOnly, allObjects bool) []map[string]any {
	filtered := make([]map[string]any, 0, len(objects))
	for _, obj := range objects {
		isBuiltIn := storage.IsBuiltIn(obj)
		isInternal := isInternalObject(obj)
		if shouldIncludeLifecycle(isBuiltIn, isInternal, builtInOnly, internalOnly, allObjects) {
			filtered = append(filtered, obj)
		}
	}
	return filtered
}

func scanLifecycleDirectory(lifecyclesDir string, builtInOnly, internalOnly, allObjects bool) ([]map[string]any, error) {
	if _, err := fileutil.Stat(lifecyclesDir); err != nil {
		if fileutil.IsNotExist(err) {
			return []map[string]any{}, nil
		}
		return nil, errfmt.Newf("failed to access lifecycles directory").Wrap(err)
	}

	var lifecycleObjects []map[string]any
	walkErr := filepath.Walk(lifecyclesDir, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil || info.IsDir() || appledouble.SkipPathInTreeWalk(path) {
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
		sourceType := internalSourceInternal
		if isBuiltIn {
			sourceType = internalSourceBuiltIn
		}

		if !shouldIncludeLifecycle(isBuiltIn, !isBuiltIn, builtInOnly, internalOnly, allObjects) {
			return nil
		}

		obj := map[string]any{
			objects.FieldKeyID:         id,
			objects.FieldKeyKind:       internalKindLifecycle,
			objects.FieldKeyTitle:      fmt.Sprintf("%s Lifecycle", objectType),
			objects.FieldKeyObjectType: objectType,
			objects.FieldKeyFilePath:   path,
			internalSourceType:         sourceType,
		}

		if schemaVersion, ok := lifecycleDef[objects.FieldKeySchemaVersion].(string); ok {
			obj[objects.FieldKeySchemaVersion] = schemaVersion
		}

		lifecycleObjects = append(lifecycleObjects, obj)
		return nil
	})

	if walkErr != nil {
		return nil, errfmt.Newf("failed to scan lifecycle directory").Wrap(walkErr)
	}

	return lifecycleObjects, nil
}

// listLifecycleDefinitionsForAll returns lifecycle definitions as QueryResult (for use in listAllInternalObjects)
func listLifecycleDefinitionsForAll(cmd *cobra.Command, proc *cli.Processor, storageProvider storage.ObjectStorageProvider, projectRoot string, builtInOnly, internalOnly, allObjects bool) (*storage.QueryResult, error) {
	_ = cmd // Reserved for future use (e.g., flag parsing)
	result, err := queryStorageInternalKind(proc, storageProvider, internalKindLifecycle)
	if err == nil && len(result.Objects) > 0 {
		filtered := filterStorageLifecycles(result.Objects, builtInOnly, internalOnly, allObjects)
		result.Objects = filtered
		result.Meta["total_count"] = len(filtered)
		return result, nil
	}

	lifecyclesDir := filepath.Join(projectRoot, paths.ProcessInternalLifecyclesDir)
	lifecycleObjects, scanErr := scanLifecycleDirectory(lifecyclesDir, builtInOnly, internalOnly, allObjects)
	if scanErr != nil {
		return nil, scanErr
	}

	return &storage.QueryResult{
		Objects: lifecycleObjects,
		Meta:    map[string]any{"total_count": len(lifecycleObjects)},
	}, nil
}

// listLifecycleDefinitions lists lifecycle definitions from file or graph backend
func listLifecycleDefinitions(cmd *cobra.Command, proc *cli.Processor, storageProvider storage.ObjectStorageProvider, projectRoot string, builtInOnly, internalOnly, allObjects bool, groupBy string, idsOnly bool) error {
	result, err := listLifecycleDefinitionsForAll(cmd, proc, storageProvider, projectRoot, builtInOnly, internalOnly, allObjects)
	if err != nil {
		return err
	}

	if groupBy != emptyValue {
		result = groupResults(result, groupBy)
	}

	outputList(cmd, result, proc.Format(), internalKindLifecycle, idsOnly)
	return nil
}
