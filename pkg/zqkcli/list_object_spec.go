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

// listObjectSpecsForAll returns object specifications as QueryResult (for use in listAllInternalObjects)
func listObjectSpecsForAll(cmd *cobra.Command, proc *cli.Processor, storageProvider storage.ObjectStorageProvider, projectRoot string, builtInOnly, internalOnly, allObjects bool) (*storage.QueryResult, error) {
	_ = cmd // Reserved for future use (e.g., flag parsing)
	result, err := queryStorageInternalKind(proc, storageProvider, internalKindObjectSpec)
	if err == nil && len(result.Objects) > 0 {
		// Graph backend has object_spec objects stored as nodes
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
	result, err = ScanObjectSpecsFromFiles(projectRoot)
	if err != nil {
		return nil, err
	}
	if allObjects || (!builtInOnly && !internalOnly) {
		return result, nil
	}
	filtered := make([]map[string]any, 0, len(result.Objects))
	for _, obj := range result.Objects {
		isBuiltIn := obj[internalSourceType] == internalSourceBuiltIn
		isInternal := obj[internalSourceType] == internalSourceInternal
		if (builtInOnly && isBuiltIn) || (internalOnly && isInternal) {
			filtered = append(filtered, obj)
		}
	}
	result.Objects = filtered
	result.Meta["total_count"] = len(filtered)
	return result, nil
}

// ScanObjectSpecsFromFiles scans .zqk/specs/objects for YAML spec files.
func ScanObjectSpecsFromFiles(projectRoot string) (*storage.QueryResult, error) {
	specsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	if _, err := fileutil.Stat(specsDir); err != nil {
		if fileutil.IsNotExist(err) {
			return &storage.QueryResult{
				Objects: []map[string]any{},
				Meta:    map[string]any{"total_count": 0},
			}, nil
		}
		return nil, errfmt.Newf("failed to access object_specs directory").Wrap(err)
	}

	var specObjects []map[string]any
	err := filepath.Walk(specsDir, func(path string, info fileutil.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return nil
		}
		if appledouble.SkipPathInTreeWalk(path) {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".yaml") && !strings.HasSuffix(info.Name(), ".yml") {
			return nil
		}
		if strings.HasPrefix(info.Name(), "_") {
			return nil
		}

		specDef, loadErr := loadYAMLDoc(path)
		if loadErr != nil {
			return nil
		}

		ontology, _ := specDef[objects.FieldKeyOntology].(string)
		if ontology == emptyValue {
			return nil
		}

		id := strings.TrimSuffix(info.Name(), filepath.Ext(info.Name()))
		title := fmt.Sprintf("%s Specification", ontology)
		if desc, ok := specDef[objects.FieldKeyDescription].(string); ok && desc != emptyValue {
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

		obj := map[string]any{
			objects.FieldKeyID:       id,
			objects.FieldKeyKind:     internalKindObjectSpec,
			objects.FieldKeyTitle:    title,
			objects.FieldKeyOntology: ontology,
			objects.FieldKeyFilePath: path,
		}

		if schemaVersion, ok := specDef[objects.FieldKeySchemaVersion].(string); ok {
			obj[objects.FieldKeySchemaVersion] = schemaVersion
		}

		if visibility, ok := specDef[objects.FieldKeyVisibility].(string); ok {
			obj[objects.FieldKeyVisibility] = visibility
		}

		isBuiltIn := strings.Contains(path, "/built-in/")
		visibility, _ := specDef[objects.FieldKeyVisibility].(string)
		isInternal := visibility == internalSourceInternal

		if isBuiltIn {
			obj[internalSourceType] = internalSourceBuiltIn
		} else if isInternal {
			obj[internalSourceType] = internalSourceInternal
		} else {
			obj[internalSourceType] = internalSourcePublic
		}

		specObjects = append(specObjects, obj)
		return nil
	})

	if err != nil {
		return nil, errfmt.Newf("failed to scan object_specs directory").Wrap(err)
	}

	return &storage.QueryResult{
		Objects: specObjects,
		Meta:    map[string]any{"total_count": len(specObjects)},
	}, nil
}

// listObjectSpecs lists object specifications from file or graph backend
func listObjectSpecs(cmd *cobra.Command, proc *cli.Processor, storageProvider storage.ObjectStorageProvider, projectRoot string, builtInOnly, internalOnly, allObjects bool, groupBy string, idsOnly bool) error {
	// Use the helper function to get object specs
	result, err := listObjectSpecsForAll(cmd, proc, storageProvider, projectRoot, builtInOnly, internalOnly, allObjects)
	if err != nil {
		return err
	}

	// Apply grouping if requested
	if groupBy != emptyValue {
		result = groupResults(result, groupBy)
	}

	// Output results
	format := proc.Format()
	outputList(cmd, result, format, internalKindObjectSpec, idsOnly)

	return nil
}
