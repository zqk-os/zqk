package object

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// ListFlags is an alias for the shared QueryFlags for backward compatibility
type ListFlags = clipkg.QueryFlags

// parseListFlags parses all list command flags using shared utilities
// Adds object-specific logic: dynamic synonym resolution and quiet profile filters
func parseListFlags(cmd *cobra.Command, proc *cli.Processor) (*ListFlags, NamespaceQueryScope, error) {
	// Use shared flag parsing utility (pass proc as Logger interface)
	flags, err := clipkg.ParseQueryFlags(cmd, proc.Logger(), ParseFilterString)
	if err != nil {
		return nil, NamespaceQueryScope{}, err
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
				return nil, NamespaceQueryScope{}, errfmt.Errorf("failed to resolve dynamic synonym '%s' for field '%s': %w", strValue, fieldName, err)
			}
			flags.Filters[fieldName] = resolvedValue
		}
	}

	// Object-specific: Apply quiet profile filters (exclude terminal statuses)
	applyQuietProfileFilters(cmd, flags.Filters)

	// Object-specific: Enforce context isolation boundaries
	scope := applyNamespaceBoundaries(cmd, flags.Filters)

	return flags, scope, nil
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
