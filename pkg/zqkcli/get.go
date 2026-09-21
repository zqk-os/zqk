package internal

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// NewInternalGetCmd creates a get command for internal objects
func NewInternalGetCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Get an internal or built-in object by ID",
		"Get an internal or built-in object by ID.",
		"",
		"This command allows reading built-in objects that may not be accessible",
		"through regular commands. It also supports lifecycle definitions and object",
		"specifications by their ID or ontology name.",
	).
		AddExample("Get a built-in component type", "%s internal get COMP-TYPE-001").
		AddExample("Get a lifecycle definition", "%s internal get backlog_item_lifecycle").
		AddExample("Get an object specification", "%s internal get backlog_item").
		AddExample("Get with JSON output", "%s internal get COMP-TYPE-001 --format json").
		ExcludeCommonFlags()

	cmd := new(cobra.Command)
	cmd.Use = "get <id>"
	cmd.Args = cobra.ExactArgs(1)
	cmd.RunE = runInternalGet

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cli.AddCommonFlags(cmd)
	return cmd
}

func runInternalGet(cmd *cobra.Command, args []string) error {
	id := args[0]

	// Create processor (handles context, storage, security, logging)
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return errfmt.Newf("failed to create processor").Wrap(err)
	}

	projectRoot := proc.ProjectRoot()

	// Check if this is a lifecycle definition ID (ends with _lifecycle)
	if strings.HasSuffix(id, "_lifecycle") {
		return getLifecycleDefinition(cmd, proc, projectRoot, id)
	}

	// Try to get it as an object_spec first (before trying regular object lookup)
	// This allows specs to be retrieved by their ontology name (e.g., "backlog_item")
	if err := getObjectSpec(cmd, proc, projectRoot, id); err == nil {
		return nil // Successfully retrieved as object spec
	}
	// If it fails, continue to regular object lookup below

	// Get appropriate storage provider (file or graph) - internal commands need graph support
	storageProvider, err := getStorageProviderForGet(projectRoot)
	if err != nil {
		logging.FluentEvent(proc.Logger()).Error("Failed to initialize storage", err).Log()
		return errfmt.Newf("failed to initialize storage").Wrap(err)
	}

	obj, err := storageProvider.Read(proc.OperationContext(), proc.SecurityContext(), id)
	if err != nil {
		storage.LogObjectReadFailure(proc.Logger(), err, id)
		return errfmt.Newf("failed to read object").Wrap(err)
	}

	// Output using format handlers for consistent formatting
	return cli.FormatOutput(cmd, obj)
}

// getStorageProviderForGet returns the appropriate storage provider (file or graph)
// This is a local version to avoid import cycles
func getStorageProviderForGet(projectRoot string) (storage.ObjectStorageProvider, error) {
	graphManager := mcp.GetGraphConnectionManager()
	if graphManager.IsEnabled() {
		// Get connection pool from graph manager
		ctx := pkgctx.NewSystemContext()
		pool, err := graphManager.GetPool(ctx)
		if err != nil {
			// If graph connection fails, fall back to file storage
			return storage.NewFileObjectStorage(projectRoot)
		}

		// Return pool-aware graph storage wrapper
		return storage.NewPoolAwareGraphStorage(pool, projectRoot), nil
	}
	// Use file-based storage
	return storage.NewFileObjectStorage(projectRoot)
}

// getLifecycleDefinition retrieves a lifecycle definition by ID
func getLifecycleDefinition(cmd *cobra.Command, proc *cli.Processor, projectRoot string, id string) error {
	// Try graph backend first
	storageProvider, err := getStorageProviderForGet(projectRoot)
	if err == nil {
		storageCtx := proc.StorageContext()

		// Try to query lifecycle objects from storage
		filter := storage.ListFilter{
			Kind:    internalKindLifecycle,
			Filters: make(map[string]any),
		}

		result, err := storageProvider.List(proc.OperationContext(), proc.SecurityContext(), storageCtx, filter)
		if err == nil && len(result.Objects) > 0 {
			// Graph backend has lifecycle objects - find the one with matching ID
			for _, obj := range result.Objects {
				if objID, _ := obj[objects.FieldKeyID].(string); objID == id {
					// Output using format handlers for consistent formatting
					return cli.FormatOutput(cmd, obj)
				}
			}
		}
	}

	// Fall back to file-based lookup
	lifecyclesDir := filepath.Join(projectRoot, paths.ProcessInternalLifecyclesDir)

	// Try to find the lifecycle file
	// ID format: decision_lifecycle -> decision_lifecycle.yaml
	lifecycleFile := id + ".yaml"
	lifecyclePath := filepath.Join(lifecyclesDir, lifecycleFile)

	// Also check built-in subdirectory
	if _, err := fileutil.Stat(lifecyclePath); fileutil.IsNotExist(err) {
		builtInPath := filepath.Join(lifecyclesDir, internalSourceBuiltIn, lifecycleFile)
		if _, err := fileutil.Stat(builtInPath); err == nil {
			lifecyclePath = builtInPath
		} else {
			return errfmt.Errorf("lifecycle definition not found: %s", id)
		}
	}

	lifecycleDef, err := loadYAMLDoc(lifecyclePath)
	if err != nil {
		logging.FluentEvent(proc.Logger()).Error(fmt.Sprintf("Failed to load lifecycle file: %s", lifecyclePath), err).
			Path(lifecyclePath).
			Log()
		if fileutil.IsNotExist(err) {
			return errfmt.Newf("failed to read lifecycle file").Wrap(err)
		}
		return errfmt.Newf("failed to parse lifecycle file").Wrap(err)
	}

	// Extract object_type
	objectType, _ := lifecycleDef[objects.FieldKeyObjectType].(string)
	if objectType == emptyValue {
		return errfmt.Errorf("lifecycle file missing object_type: %s", lifecyclePath)
	}

	// Build object representation (similar to listLifecycleDefinitionsForAll)
	obj := map[string]any{
		objects.FieldKeyID:         id,
		objects.FieldKeyKind:       internalKindLifecycle,
		objects.FieldKeyTitle:      fmt.Sprintf("%s Lifecycle", objectType),
		objects.FieldKeyObjectType: objectType,
		objects.FieldKeyFilePath:   lifecyclePath,
	}

	if schemaVersion, ok := lifecycleDef[objects.FieldKeySchemaVersion].(string); ok {
		obj[objects.FieldKeySchemaVersion] = schemaVersion
	}

	// Add metadata if present
	if metadata, ok := lifecycleDef[objects.FieldKeyMetadata].(map[string]any); ok {
		obj[objects.FieldKeyMetadata] = metadata
	}

	// Check if this is a built-in lifecycle
	isBuiltIn := strings.Contains(lifecyclePath, "/built-in/")
	if isBuiltIn {
		obj[objects.FieldKeySourceType] = internalSourceBuiltIn
	} else {
		obj[objects.FieldKeySourceType] = internalSourceInternal
	}

	// Include the full lifecycle definition
	obj["lifecycle_definition"] = lifecycleDef

	// Output using format handlers for consistent formatting
	return cli.FormatOutput(cmd, obj)
}

// getObjectSpec retrieves an object specification by ID
func getObjectSpec(cmd *cobra.Command, proc *cli.Processor, projectRoot string, id string) error {
	// Try graph backend first
	storageProvider, err := getStorageProviderForGet(projectRoot)
	if err == nil {
		storageCtx := proc.StorageContext()

		// Try to query object_spec objects from storage
		filter := storage.ListFilter{
			Kind:    internalKindObjectSpec,
			Filters: make(map[string]any),
		}

		result, err := storageProvider.List(proc.OperationContext(), proc.SecurityContext(), storageCtx, filter)
		if err == nil && len(result.Objects) > 0 {
			// Graph backend has object_spec objects - find the one with matching ID
			for _, obj := range result.Objects {
				if objID, _ := obj[objects.FieldKeyID].(string); objID == id {
					// Output using format handlers for consistent formatting
					return cli.FormatOutput(cmd, obj)
				}
			}
		}
	}

	// Fall back to file-based lookup
	specsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)

	// Try to find the spec file
	// ID format: backlog_item -> backlog_item.yaml
	specFile := id + ".yaml"
	specPath := filepath.Join(specsDir, specFile)

	// Also check built-in subdirectory
	if _, err := fileutil.Stat(specPath); fileutil.IsNotExist(err) {
		builtInPath := filepath.Join(specsDir, internalSourceBuiltIn, specFile)
		if _, err := fileutil.Stat(builtInPath); err == nil {
			specPath = builtInPath
		} else {
			// Not an object spec, return error so it can try regular object lookup
			return errfmt.Errorf("object spec not found: %s", id)
		}
	}

	specDef, err := loadYAMLDoc(specPath)
	if err != nil {
		logging.FluentEvent(proc.Logger()).Error(fmt.Sprintf("Failed to load spec file: %s", specPath), err).
			Path(specPath).
			Log()
		if fileutil.IsNotExist(err) {
			return errfmt.Newf("failed to read spec file").Wrap(err)
		}
		return errfmt.Newf("failed to parse spec file").Wrap(err)
	}

	// Extract ontology
	ontology, _ := specDef[objects.FieldKeyOntology].(string)
	if ontology == emptyValue {
		return errfmt.Errorf("spec file missing ontology: %s", specPath)
	}

	// Build object representation
	obj := map[string]any{
		objects.FieldKeyID:       id,
		objects.FieldKeyKind:     internalKindObjectSpec,
		objects.FieldKeyTitle:    fmt.Sprintf("%s Specification", ontology),
		objects.FieldKeyOntology: ontology,
		objects.FieldKeyFilePath: specPath,
	}

	if schemaVersion, ok := specDef[objects.FieldKeySchemaVersion].(string); ok {
		obj[objects.FieldKeySchemaVersion] = schemaVersion
	}

	if visibility, ok := specDef[objects.FieldKeyVisibility].(string); ok {
		obj[objects.FieldKeyVisibility] = visibility
	}

	// Check if this is a built-in spec
	isBuiltIn := strings.Contains(specPath, "/built-in/")
	if isBuiltIn {
		obj[objects.FieldKeySourceType] = internalSourceBuiltIn
	} else if visibility, ok := specDef[objects.FieldKeyVisibility].(string); ok && visibility == internalSourceInternal {
		obj[objects.FieldKeySourceType] = internalSourceInternal
	} else {
		obj[objects.FieldKeySourceType] = internalSourcePublic
	}

	// Include the full spec definition
	obj["spec_definition"] = specDef

	// Output using format handlers for consistent formatting
	return cli.FormatOutput(cmd, obj)
}
