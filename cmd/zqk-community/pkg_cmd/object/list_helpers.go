package object

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/spf13/cobra"
)

// ListFlags is an alias for the shared QueryFlags for backward compatibility
type ListFlags = clipkg.QueryFlags

// parseListFlags parses all list command flags using shared utilities
// Adds object-specific logic: dynamic synonym resolution and quiet profile filters
func parseListFlags(cmd *cobra.Command, proc *cli.Processor) (*ListFlags, error) {
	// Use shared flag parsing utility (pass proc as Logger interface)
	flags, err := clipkg.ParseQueryFlags(cmd, proc.Logger(), ParseFilterString)
	if err != nil {
		return nil, err
	}

	// Object-specific: Resolve dynamic synonyms (e.g., "current" for priority_plan_ref)
	for fieldName, filterValue := range flags.Filters {
		if strValue, ok := filterValue.(string); ok {
			resolvedValue, err := resolveDynamicSynonym(fieldName, strValue, proc.OperationContext(), proc.Storage())
			if err != nil {
				logging.FluentEvent(proc.Logger()).Error("Failed to resolve dynamic synonym", err).
					String("field", fieldName).
					String("value", strValue).
					Log()
				return nil, errfmt.Errorf("failed to resolve dynamic synonym '%s' for field '%s': %w", strValue, fieldName, err)
			}
			flags.Filters[fieldName] = resolvedValue
		}
	}

	// Object-specific: Apply quiet profile filters (exclude terminal statuses)
	applyQuietProfileFilters(cmd, flags.Filters)

	// Object-specific: Enforce context isolation boundaries
	applyNamespaceBoundaries(cmd, flags.Filters)

	return flags, nil
}

// applyNamespaceBoundaries enforces strict workspace context isolation.
// If no explicit namespace filter is provided via flags (e.g. `--filter namespace_id=tenant:sandbox`),
// the query is scoped down to the default system kernel namespace to prevent workload contamination.
func applyNamespaceBoundaries(cmd *cobra.Command, filters map[string]any) {
	if filters == nil {
		return
	}

	// If --federated is provided, allow cross-namespace querying
	if federated, err := cmd.Flags().GetBool("federated"); err == nil && federated {
		return
	}

	// If --namespace is explicitly provided, use it
	if ns, err := cmd.Flags().GetString("namespace"); err == nil && ns != "" {
		filters[objects.FieldKeyNamespaceID] = ns
		return
	}

	// Check if the user has explicitly requested a specific namespace via --filter namespace_id=...
	if _, hasNamespace := filters[objects.FieldKeyNamespaceID]; hasNamespace {
		return
	}

	// Enforce strict local boundary.
	filters[objects.FieldKeyNamespaceID] = validation.DefaultNamespaceKernel
}

// buildListFilter builds a ListFilter from flags and kind using shared utilities.
// When the user omits --limit (flags.Limit == 0), we force filter.Limit to 0 so storage returns the full list
// regardless of storage context (MaxPageSize/DefaultPageSize).
func buildListFilter(kind string, flags *ListFlags, storageCtx *storage.StorageContext) storage.ListFilter {
	filterData := clipkg.BuildListFilterData(kind, flags, storageCtx.MaxPageSize)
	limit := filterData["limit"].(int)
	if flags.Limit == 0 {
		limit = 0
	}
	fields, _ := filterData["fields"].([]string)

	return storage.ListFilter{
		Kind:    filterData[objects.FieldKeyKind].(string),
		Filters: filterData["filters"].(map[string]any),
		SortBy:  filterData["sort_by"].(string),
		SortAsc: filterData["sort_asc"].(bool),
		Offset:  filterData["offset"].(int),
		Limit:   limit,
		GroupBy: filterData["group_by"].(string),
		Fields:  fields,
	}
}

// listTableFieldOrder returns column order for table output when --fields is set (matches storage list projection ordering).
func listTableFieldOrder(flags *ListFlags) []string {
	if flags == nil || len(flags.Fields) == 0 {
		return nil
	}
	names := objects.ListProjectionFieldNames(flags.Fields, flags.SortBy)
	names = objects.ListProjectionFieldNames(names, flags.GroupBy)
	return names
}

// calculateEffectiveLimitFromContext calculates effective limit for list commands.
// When the user omits --limit (flagLimit 0), returns 0 so cacheable lists get the full cached set.
func calculateEffectiveLimitFromContext(flagLimit int, storageCtx *storage.StorageContext) int {
	if flagLimit == 0 {
		return 0
	}
	return clipkg.BuildListFilterData("", &clipkg.QueryFlags{Limit: flagLimit}, storageCtx.MaxPageSize)["limit"].(int)
}

// outputListResults outputs list results based on format.
// When tableFieldOrder is non-empty (user specified --fields), table columns match that projection order.
func outputListResults(cmd *cobra.Command, proc *cli.Processor, result *storage.QueryResult, kind, groupBy string, countOnly bool, tableFieldOrder []string) error {
	format := proc.Format()

	if countOnly {
		countResult := &clipkg.CountResult{
			Objects: result.Objects,
			Groups:  result.Groups,
			Meta:    result.Meta,
		}
		data, err := clipkg.OutputCount(countResult, string(format), kind, groupBy)
		if err != nil {
			return err
		}
		return cli.WriteOutput(cmd, data)
	}

	switch format {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return outputListStructured(cmd, result)
	case cli.FormatTable:
		if kind == emptyValue {
			return outputListTableGroupedByKind(cmd, result, tableFieldOrder)
		}
		return outputListTable(cmd, result, kind, groupBy, tableFieldOrder)
	default:
		if kind == emptyValue {
			return outputListTableGroupedByKind(cmd, result, tableFieldOrder)
		}
		return outputListTable(cmd, result, kind, groupBy, tableFieldOrder)
	}
}
