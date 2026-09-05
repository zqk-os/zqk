package mcp

import (
	"context"
	"errors"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"gopkg.in/yaml.v3"
)

// ErrStoredMCPSpecUnusable marks persisted configuration that exists but
// cannot be safely selected. Callers must not silently downgrade to fallback.
var ErrStoredMCPSpecUnusable = errors.New("stored mcp_spec is unusable")

// StorageMCPSpecLoaderInterface is an interface for loading MCP specs from storage
// This allows the MCP server to load specs without directly importing storage
// Note: MCPSpecLoader is already defined in mcp_spec_generator.go for file-based loading
type StorageMCPSpecLoaderInterface interface {
	LoadMCPSpecs(ctx context.Context, secCtx *pkgctx.SecurityContext) ([]*MCPSpec, error)
}

// StorageMCPSpecLoader implements StorageMCPSpecLoaderInterface using StorageProvider
// This leverages existing CLI/storage infrastructure instead of duplicating file system logic
type StorageMCPSpecLoader struct {
	storageProvider StorageProvider
}

// NewStorageMCPSpecLoader creates a new storage-based MCP spec loader
func NewStorageMCPSpecLoader(provider StorageProvider) *StorageMCPSpecLoader {
	return &StorageMCPSpecLoader{
		storageProvider: provider,
	}
}

// LoadMCPSpecs loads all mcp_spec objects from storage
// This discovers specs as formal system objects, allowing them to be managed via CLI
func (loader *StorageMCPSpecLoader) LoadMCPSpecs(ctx context.Context, secCtx *pkgctx.SecurityContext) ([]*MCPSpec, error) {
	if loader.storageProvider == nil {
		return nil, errfmt.Errorf("storage provider not set")
	}

	storageCtx := pkgctx.NewStorageContext()

	// Use ListWithConfig helper if available, otherwise use direct List
	// Create a filter for mcp_spec objects
	filter := map[string]any{
		objects.FieldKeyKind:   objects.KindMcpSpec,
		objects.FieldKeyStatus: objects.ObjectStatusActive,
	}

	result, err := loader.storageProvider.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return nil, errfmt.Newf("failed to list mcp_spec objects").Wrap(err)
	}

	// Adapt result to our interface
	adapter := AdaptStorageResult(result)
	if adapter == nil || len(adapter.Objects) == 0 {
		return []*MCPSpec{}, nil
	}

	// Convert storage objects to MCPSpec
	specs := make([]*MCPSpec, 0, len(adapter.Objects))
	for _, obj := range adapter.Objects {
		spec, err := loader.convertObjectToSpec(obj)
		if err != nil {
			return nil, errfmt.Errorf(
				"%w: object id=%q name=%q: %v",
				ErrStoredMCPSpecUnusable,
				objects.GetString(obj, objects.FieldKeyID),
				objects.GetString(obj, objects.FieldKeyName),
				err,
			)
		}
		if spec != nil {
			specs = append(specs, spec)
		}
	}

	return specs, nil
}

// convertObjectToSpec converts a storage object to MCPSpec
// The object should have a "spec" field containing the YAML spec data
func (loader *StorageMCPSpecLoader) convertObjectToSpec(obj map[string]any) (*MCPSpec, error) {
	// Extract spec data - can be in "spec" field (YAML string) or directly as object fields
	var specData map[string]any

	// Try "spec" field first (YAML string)
	if specStr, ok := obj[objects.FieldKeySpec].(string); ok {
		if err := yaml.Unmarshal([]byte(specStr), &specData); err != nil {
			return nil, errfmt.Newf("failed to unmarshal spec YAML").Wrap(err)
		}
	} else if specMap, ok := obj[objects.FieldKeySpec].(map[string]any); ok {
		// Spec is already a map
		specData = specMap
	} else {
		// Try to use object fields directly (for objects that store spec inline)
		specData = obj
	}

	// Marshal to YAML bytes then unmarshal to MCPSpec struct
	// This handles type conversions properly
	yamlBytes, err := yaml.Marshal(specData)
	if err != nil {
		return nil, errfmt.Newf("failed to marshal spec data").Wrap(err)
	}

	var spec MCPSpec
	if err := yaml.Unmarshal(yamlBytes, &spec); err != nil {
		return nil, errfmt.Newf("failed to unmarshal spec").Wrap(err)
	}

	// Validate spec
	if err := spec.Validate(); err != nil {
		return nil, errfmt.Newf("spec validation failed").Wrap(err)
	}
	if objectName := objects.GetString(obj, objects.FieldKeyName); objectName != emptyValue && objectName != spec.Name {
		return nil, errfmt.Errorf("mcp_spec object name %q does not match payload name %q", objectName, spec.Name)
	}

	return &spec, nil
}

// LoadMCPSpecsWithFilter loads MCP specs with additional filtering
// This allows filtering by name, category, or other fields
func (loader *StorageMCPSpecLoader) LoadMCPSpecsWithFilter(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	nameFilter string,
) ([]*MCPSpec, error) {
	specs, err := loader.LoadMCPSpecs(ctx, secCtx)
	if err != nil {
		return nil, err
	}

	if nameFilter == emptyValue {
		return specs, nil
	}

	// Filter by name
	filtered := make([]*MCPSpec, 0)
	for _, spec := range specs {
		if spec.Name == nameFilter {
			filtered = append(filtered, spec)
		}
	}
	if len(filtered) > 1 {
		return nil, errfmt.Errorf(
			"%w: multiple active mcp_spec objects named %q",
			ErrStoredMCPSpecUnusable,
			nameFilter,
		)
	}

	return filtered, nil
}
