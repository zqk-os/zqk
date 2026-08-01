package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/paths"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/appledouble"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// listObjectSpecsForAll returns object specifications as QueryResult (for use in listAllInternalObjects)
func listObjectSpecsForAll(cmd *cobra.Command, proc *cli.Processor, storageProvider storage.ObjectStorageProvider, projectRoot string, builtInOnly, internalOnly, allObjects bool) (*storage.QueryResult, error) {
	_ = cmd // Reserved for future use (e.g., flag parsing)
	// Check if this is a graph backend by trying to query for object_spec objects
	secCtx := proc.SecurityContext()
	storageCtx := proc.StorageContext()

	// Try to query object_spec objects from storage
	filter := storage.ListFilter{
		Kind:    internalKindObjectSpec,
		Filters: make(map[string]any),
	}

	result, err := storageProvider.List(proc.OperationContext(), secCtx, storageCtx, filter)
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
	specsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	if _, err := os.Stat(specsDir); err != nil {
		if os.IsNotExist(err) {
			return &storage.QueryResult{
				Objects: []map[string]any{},
				Meta:    map[string]any{"total_count": 0},
			}, nil
		}
		return nil, errfmt.Newf("failed to access object_specs directory").Wrap(err)
	}

	var specObjects []map[string]any
	err = filepath.Walk(specsDir, func(path string, info os.FileInfo, walkErr error) error {
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

		// Skip placeholder files
		if strings.HasPrefix(info.Name(), "_") {
			return nil
		}

		// Read spec file
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}

		var specDef map[string]any
		if parseErr := yaml.Unmarshal(data, &specDef); parseErr != nil {
			return nil
		}

		// Extract ontology (which is the kind this spec defines)
		ontology, _ := specDef[objects.FieldKeyOntology].(string)
		if ontology == emptyValue {
			return nil
		}

		// Create an object-like structure for display
		id := strings.TrimSuffix(info.Name(), filepath.Ext(info.Name()))

		// Extract title from description or use ontology
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

		// Build object representation
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

		// Check if this is a built-in spec or internal
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

		// Apply filters
		shouldInclude := false
		if allObjects {
			shouldInclude = true
		} else if builtInOnly {
			shouldInclude = isBuiltIn
		} else if internalOnly {
			shouldInclude = isInternal
		} else {
			shouldInclude = true
		}

		if shouldInclude {
			specObjects = append(specObjects, obj)
		}

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
