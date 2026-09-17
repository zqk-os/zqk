package internal

import (
	"strings"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/spf13/cobra"

	"github.com/lanceman/zqk/pkg/objects"
)

// CountFlags contains parsed count command flags
type CountFlags struct {
	Filters          map[string]any
	BuiltInOnly      bool
	InternalOnly     bool
	AllObjects       bool
	GroupBy          string
	IncludeZeroCount bool // When true, show kinds with 0 count; default false omits them to reduce noise
}

// parseCountFlags parses all count command flags
func parseCountFlags(cmd *cobra.Command, proc *cli.Processor) (*CountFlags, error) {
	flags := &CountFlags{
		Filters: make(map[string]any),
	}

	var flagsBag clipkg.FlagBag
	flags.BuiltInOnly = flagsBag.Bool(cmd, "built-in")
	flags.InternalOnly = flagsBag.Bool(cmd, "internal")
	flags.AllObjects = flagsBag.Bool(cmd, "all")
	flags.GroupBy = flagsBag.String(cmd, "group-by")
	flags.IncludeZeroCount = flagsBag.Bool(cmd, "include-zero-count")

	// Build filters using shared utility
	filterStrs := flagsBag.StringArray(cmd, "filter")
	if err := flagsBag.Err(); err != nil {
		return nil, err
	}

	for _, filterStr := range filterStrs {
		fieldName, filterValue, err := clipkg.ParseFilterString(filterStr)
		if err != nil {
			logging.FluentEvent(proc.Logger()).Error("Failed to parse filter", err).
				String("filter", filterStr).
				Log()
			return nil, err
		}
		flags.Filters[fieldName] = filterValue
	}

	return flags, nil
}

// buildCountFilter builds a ListFilter for counting
func buildCountFilter(kind string, flags *CountFlags) storage.ListFilter {
	filters := make(map[string]any)
	for k, v := range flags.Filters {
		filters[k] = v
	}

	// Apply built-in/internal filtering
	if flags.BuiltInOnly {
		filters["built_in"] = true
	}
	if flags.InternalOnly {
		filters[objects.FieldKeyVisibility] = internalSourceInternal
	}

	return storage.ListFilter{
		Kind:    kind,
		Filters: filters,
		GroupBy: flags.GroupBy,
	}
}

// shouldUseListForCount determines if List() should be used instead of Count()
// Returns true when we need to filter objects in memory (e.g., when filtering for built_in OR internal without specific flags)
// Returns false when filters can be pushed to storage level (e.g., BuiltInOnly or InternalOnly flags)
func shouldUseListForCount(flags *CountFlags) bool {
	// If user explicitly set BuiltInOnly or InternalOnly, we can use Count() with storage-level filters
	// The buildCountFilter function already adds these filters to the ListFilter
	if flags.BuiltInOnly || flags.InternalOnly {
		// Only need List() if there are other filters that require object inspection
		return len(flags.Filters) > 0
	}

	// If AllObjects is true, we can use Count() unless there are filters
	if flags.AllObjects {
		return len(flags.Filters) > 0
	}

	// When !AllObjects and no explicit flags, we need to filter for (built_in OR internal)
	// This requires loading objects to check both conditions, so use List()
	return true
}

// filterObjectsByVisibility filters objects by visibility based on flags
func filterObjectsByVisibility(objectList []map[string]any, flags *CountFlags) []map[string]any {
	if flags.AllObjects {
		return objectList
	}

	filtered := make([]map[string]any, 0, len(objectList))
	for _, obj := range objectList {
		isBuiltIn := storage.IsBuiltIn(obj)
		isInternal := isInternalObject(obj)

		if isBuiltIn || isInternal {
			filtered = append(filtered, obj)
		}
	}
	return filtered
}

// getCountFromResult extracts count from a QueryResult
func getCountFromResult(result *storage.QueryResult) int {
	if total, ok := result.Meta["total_count"].(int); ok {
		return total
	}
	return len(result.Objects)
}

// parseKindsFromArg parses comma-separated kinds from an argument and validates each against
// synonym mappings and the project's spec index (when present).
func parseKindsFromArg(projectRoot, kindArg string) ([]string, error) {
	if !strings.Contains(kindArg, ",") {
		k, err := objects.ResolveAndValidateKindForProject(projectRoot, kindArg)
		if err != nil {
			return nil, err
		}
		return []string{k}, nil
	}
	return objects.ResolveAndValidateKindsCommaSeparated(projectRoot, kindArg)
}

// filterInternalKinds filters a list of kinds to only include internal kinds
func filterInternalKinds(allKinds []string) []string {
	internalKinds := make([]string, 0, len(allKinds))
	for _, kind := range allKinds {
		if isInternalKind(kind) {
			internalKinds = append(internalKinds, kind)
		}
	}
	return internalKinds
}

// isInternalKind checks if a kind has visibility: internal in its spec
// This is a copy of the function from cmd/zqk/object/list.go to avoid cross-package dependency
func isInternalKind(kind string) bool {
	// Check known internal kinds first (fast path)
	knownInternalKinds := map[string]bool{
		objects.KindAuditEvent:         true,
		objects.KindChangeJournalEntry: true,
		internalKindLifecycle:          true,
		internalKindObjectSpec:         true,
		objects.KindTemplate:           true,
		objects.KindIntegrityManifest:  true,
		objects.KindSynonym:            true,
	}
	if knownInternalKinds[kind] {
		return true
	}

	// Load spec to check visibility
	specLoader := objects.NewSpecLoader("")
	specFile := kind + ".yaml"
	spec, err := specLoader.LoadSpecWithInheritance(specFile)
	if err != nil {
		// If spec can't be loaded, assume it's not internal (safer default)
		return false
	}

	// Check visibility from spec - only the spec's own visibility matters
	// Visibility is NOT inherited from parent specs
	if spec.Visibility == internalSourceInternal {
		return true
	}

	return false
}
