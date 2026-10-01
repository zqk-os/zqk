package internal

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/storage"

	"github.com/zqk-os/zqk/pkg/objects"
)

// InternalListFlags contains parsed internal list command flags (routing and visibility).
// List-style flags (filter, sort, limit, etc.) are parsed by the trait harness when using Run().
type InternalListFlags struct {
	BuiltInOnly  bool
	InternalOnly bool
	AllObjects   bool
	GroupBy      string
	IDsOnly      bool
}

// parseInternalListFlags parses internal list command flags: harness list flags via
// ParseListFlagsFromCommand (single source of truth) plus internal-specific visibility flags.
func parseInternalListFlags(cmd *cobra.Command, _ *cli.Processor) (*InternalListFlags, error) {
	listOpts, err := clipkg.ParseListFlagsFromCommand(cmd)
	if err != nil {
		return nil, err
	}
	var flagsBag clipkg.FlagBag
	flags := &InternalListFlags{
		GroupBy:      listOpts.GroupBy,
		IDsOnly:      listOpts.IdsOnly,
		BuiltInOnly:  flagsBag.Bool(cmd, "built-in"),
		InternalOnly: flagsBag.Bool(cmd, "internal"),
		AllObjects:   flagsBag.Bool(cmd, "all"),
	}
	if err := flagsBag.Err(); err != nil {
		return nil, err
	}
	return flags, nil
}

// extractKindFromArgs extracts the kind from command arguments
func extractKindFromArgs(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return ""
}

// routeToListHandler routes to the appropriate list handler based on kind
func routeToListHandler(kind string, cmd *cobra.Command, proc *cli.Processor, storageProvider storage.ObjectStorageProvider, projectRoot string, flags *InternalListFlags) error {
	// Special handling for lifecycle kind
	if kind == internalKindLifecycle {
		return listLifecycleDefinitions(cmd, proc, storageProvider, projectRoot, flags.BuiltInOnly, flags.InternalOnly, flags.AllObjects, flags.GroupBy, flags.IDsOnly)
	}

	// Special handling for object_spec kind
	if kind == internalKindObjectSpec {
		return listObjectSpecs(cmd, proc, storageProvider, projectRoot, flags.BuiltInOnly, flags.InternalOnly, flags.AllObjects, flags.GroupBy, flags.IDsOnly)
	}

	// List all internal/built-in objects across all kinds
	if kind == emptyValue {
		secCtx := proc.SecurityContext()
		storageCtx := proc.StorageContext()
		return listAllInternalObjects(cmd, proc, storageProvider, secCtx, storageCtx, flags.BuiltInOnly, flags.InternalOnly, flags.AllObjects, projectRoot, flags.GroupBy, flags.IDsOnly)
	}

	return nil // Continue with single-kind listing
}

// filterResultByVisibility filters result.Objects by built-in/internal/all. Used by both
// filterListObjectsByVisibility and internalListSource.
func filterResultByVisibility(result *storage.QueryResult, builtInOnly, internalOnly, allObjects bool) {
	if allObjects {
		return
	}
	if !builtInOnly && !internalOnly {
		// Default: show only built-in or internal
		filtered := make([]map[string]any, 0)
		for _, obj := range result.Objects {
			if storage.IsBuiltIn(obj) || isInternalObject(obj) {
				filtered = append(filtered, obj)
			}
		}
		result.Objects = filtered
		updateResultMetadata(result, len(filtered))
		return
	}
	filtered := make([]map[string]any, 0)
	for _, obj := range result.Objects {
		if builtInOnly && storage.IsBuiltIn(obj) {
			filtered = append(filtered, obj)
		} else if internalOnly && isInternalObject(obj) {
			filtered = append(filtered, obj)
		}
	}
	result.Objects = filtered
	updateResultMetadata(result, len(filtered))
}

// filterListObjectsByVisibility filters objects based on visibility flags for list command

// internalListSource implements clipkg.ListSource for single-kind internal list (trait harness).
type internalListSource struct {
	proc            *cli.Processor
	storageProvider storage.ObjectStorageProvider
	kind            string
	builtInOnly     bool
	internalOnly    bool
	allObjects      bool
}

func (s internalListSource) List(ctx context.Context, opts clipkg.ListOptions) ([]map[string]any, int, error) {
	_ = ctx // use proc.OperationContext() for storage so security/storage context are correct
	secCtx := s.proc.SecurityContext()
	storageCtx := s.proc.StorageContext()
	filter := storage.ListFilter{
		Kind:    s.kind,
		Filters: opts.Filters,
		SortBy:  opts.SortBy,
		SortAsc: opts.SortAsc,
		Offset:  opts.Offset,
		Limit:   opts.Limit,
		GroupBy: opts.GroupBy,
	}
	result, err := s.storageProvider.List(s.proc.OperationContext(), secCtx, storageCtx, filter)
	if err != nil {
		return nil, 0, err
	}
	filterResultByVisibility(result, s.builtInOnly, s.internalOnly, s.allObjects)
	return result.Objects, len(result.Objects), nil
}

// updateResultMetadata updates result metadata with filtered count
func updateResultMetadata(result *storage.QueryResult, count int) {
	if result.Meta != nil {
		result.Meta["total_count"] = count
		result.Meta["returned_count"] = count
	}
}

// applyGroupingIfRequested applies grouping to results if requested

// isInternalObject checks if an object is internal (visibility: internal)
// Internal objects are system objects, not regular user-created objects.
// They are identified by:
// 1. Their kind (lifecycle definitions, specs are always internal)
// 2. The spec's visibility field (dynamically checked)
// 3. Explicit metadata indicating they're internal system objects
//
// Note: source_type: "internal" alone is NOT sufficient - that just means
// the object originated internally, not that it's an internal/system object.
// Regular backlog items can have source_type: "internal" but are still public objects.
//
// IMPORTANT: Object specs and lifecycle definitions are ALWAYS internal system objects,
// regardless of the visibility they define for the objects they describe.
func isInternalObject(obj map[string]any) bool {
	kind, _ := obj[objects.FieldKeyKind].(string)
	if kind == emptyValue {
		return false
	}

	// Special cases: lifecycle and object_spec are always internal system objects
	// They are system objects regardless of what visibility they define for the objects they describe
	if kind == internalKindLifecycle || kind == internalKindObjectSpec {
		return true
	}

	// Check for explicit internal metadata/visibility on the object itself
	if metadata, ok := obj[objects.FieldKeyMetadata].(map[string]any); ok {
		if visibility, ok := metadata[objects.FieldKeyVisibility].(string); ok && visibility == internalSourceInternal {
			return true
		}
		// Check for explicit internal flag
		if internal, ok := metadata["internal"].(bool); ok && internal {
			return true
		}
	}

	// SpecLoader already stamp-memos YAML; do not keep a second visibility map.
	specLoader := objects.GetGlobalSpecLoader()
	specFile := kind + ".yaml"
	spec, err := specLoader.LoadSpecWithInheritance(specFile)
	if err != nil || spec == nil {
		return false
	}

	visibility := spec.Visibility
	if visibility == emptyValue && spec.Extends != emptyValue {
		parentFile := spec.Extends + ".yaml"
		if parentSpec, err := specLoader.LoadSpecWithInheritance(parentFile); err == nil && parentSpec != nil {
			visibility = parentSpec.Visibility
		}
	}

	return visibility == internalSourceInternal
}

// getStorageProvider returns the appropriate storage provider (file or graph)
// For graph backend, uses a pool-aware wrapper that works with the connection pool
func getStorageProvider(projectRoot string) (storage.ObjectStorageProvider, error) {
	graphManager := mcp.GetGraphConnectionManager()
	if graphManager.IsEnabled() {
		// Get connection pool from graph manager
		ctx := pkgctx.NewSystemContext()
		pool, err := graphManager.GetPool(ctx)
		if err != nil {
			// If graph connection fails, fall back to file storage
			// This allows the system to work even if graph is temporarily unavailable
			return storage.NewFileObjectStorage(projectRoot)
		}

		// Return pool-aware graph storage wrapper
		return storage.NewPoolAwareGraphStorage(pool, projectRoot), nil
	}
	// Use file-based storage
	return storage.NewFileObjectStorage(projectRoot)
}
