package cli

import (
	"fmt"
	"strings"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/spf13/cobra"
)

// Logger is a minimal interface for logging errors
// This allows pkg/cli to work without importing internal/cli
type Logger interface {
	LogError(msg string, err error, fields ...logging.Field)
}

// QueryFlags contains parsed flags for list/count operations
// This is a shared structure that can be extended by specific commands
type QueryFlags struct {
	Filters    map[string]any
	SortBy     string
	SortAsc    bool
	Offset     int
	Limit      int
	GroupBy    string
	GroupLimit int
	CountOnly  bool
	// Fields lists top-level YAML keys to return per object (storage hybrid projection); empty means full maps.
	Fields []string
}

// ParseQueryFlags parses common query flags (filters, sorting, pagination, grouping)
// This is a shared utility that can be used by object, internal, and other commands
// logger can be nil if error logging is not needed
func ParseQueryFlags(cmd *cobra.Command, logger Logger, parseFilter func(string) (string, any, error)) (*QueryFlags, error) {
	flags := &QueryFlags{
		Filters: make(map[string]any),
	}

	// Parse filters
	filterStrs, err := cmd.Flags().GetStringArray("filter")
	if err != nil {
		filterStrs = []string{}
	}
	for _, filterStr := range filterStrs {
		fieldName, filterValue, err := parseFilter(filterStr)
		if err != nil {
			if logger != nil {
				if r, ok := logging.TryFluentEvent(logger); ok {
					r.Error("Failed to parse filter", err).FilterExpr(filterStr).Log()
				} else {
					logger.LogError("Failed to parse filter", err, logging.String("filter", filterStr))
				}
			}
			return nil, err
		}
		flags.Filters[fieldName] = filterValue
	}

	// Parse sorting
	if sortBy, err := cmd.Flags().GetString("sort-by"); err == nil {
		flags.SortBy = sortBy
	}
	if sortAsc, err := cmd.Flags().GetBool("sort-asc"); err == nil {
		flags.SortAsc = sortAsc
	}

	// Parse pagination
	if offset, err := cmd.Flags().GetInt("offset"); err == nil {
		flags.Offset = offset
	}
	if limit, err := cmd.Flags().GetInt("limit"); err == nil {
		flags.Limit = limit
	}

	// Parse grouping
	if groupBy, err := cmd.Flags().GetString("group-by"); err == nil {
		flags.GroupBy = groupBy
	}
	if groupLimit, err := cmd.Flags().GetInt("group-limit"); err == nil {
		flags.GroupLimit = groupLimit
	}

	// Parse count-only
	if countOnly, err := cmd.Flags().GetBool("count"); err == nil {
		flags.CountOnly = countOnly
	}

	// Parse projected fields (--fields may be repeated; each value can be comma-separated).
	fields, ferr := FieldsFromCmd(cmd)
	if ferr != nil {
		return nil, ferr
	}
	flags.Fields = fields

	return flags, nil
}

// FieldsFromCmd parses the "fields" StringArray flag (comma-split segments, dedupe, reject keys starting with '-').
// Returns nil when unset or empty (no projection).
func FieldsFromCmd(cmd *cobra.Command) ([]string, error) {
	raw, err := cmd.Flags().GetStringArray("fields")
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	return normalizeFieldsFlagValues(raw)
}

func normalizeFieldsFlagValues(raw []string) ([]string, error) {
	var out []string
	seen := make(map[string]struct{})
	for _, s := range raw {
		for _, part := range strings.Split(s, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			cleanName := part
			if name, _, ok := strings.Cut(part, ":"); ok {
				cleanName = strings.TrimSpace(name)
			}
			if cleanName == "" {
				continue
			}
			if strings.HasPrefix(cleanName, "-") {
				return nil, fmt.Errorf(
					"invalid --fields key %q (top-level YAML keys cannot start with '-'); "+
						"if you meant command help, put -h before flags that consume the next argument (e.g. \"zqk object list -h backlog_item\")",
					cleanName,
				)
			}
			if _, ok := seen[cleanName]; ok {
				continue
			}
			seen[cleanName] = struct{}{}
			out = append(out, cleanName)
		}
	}
	return out, nil
}

// QueryFlagNames returns flag names registered by [AddQueryFlags] (for spec coverage / expected locals).
func QueryFlagNames() []string {
	return []string{
		"filter",
		"sort-by",
		"sort-asc",
		"offset",
		"limit",
		"group-by",
		"group-limit",
		"count",
		"fields",
	}
}

// StorageContext represents storage context limits without importing pkg/storage
// This breaks the import cycle between pkg/cli and pkg/storage
type StorageContext struct {
	MaxPageSize     int
	DefaultPageSize int
	EnableGrouping  bool
}

// BuildListFilterData builds list filter data from QueryFlags
// Returns the filter data that can be used to construct a storage.ListFilter
// This avoids importing pkg/storage directly
func BuildListFilterData(kind string, flags *QueryFlags, maxPageSize int) map[string]any {
	effectiveLimit := calculateEffectiveLimit(flags.Limit, maxPageSize)

	return map[string]any{
		objects.FieldKeyKind: kind,
		"filters":            flags.Filters,
		"sort_by":            flags.SortBy,
		"sort_asc":           flags.SortAsc,
		"offset":             flags.Offset,
		"limit":              effectiveLimit,
		"group_by":           flags.GroupBy,
		"group_limit":        flags.GroupLimit,
		"fields":             flags.Fields,
	}
}

// calculateEffectiveLimit returns the limit to send to storage.
// 0 means no limit (omit or explicit --limit 0). A positive flag limit is passed through unchanged;
// MaxPageSize is not applied so the user's explicit limit is respected (paging can be used to reach all data).
func calculateEffectiveLimit(flagLimit, maxPageSize int) int { //nolint:unparam // maxPageSize kept for caller API; no cap applied
	if flagLimit <= 0 {
		return 0
	}
	return flagLimit
}

// AddQueryFlags adds common query flags to a command
// This standardizes flag definitions across list/count commands
func AddQueryFlags(cmd *cobra.Command) {
	cmd.Flags().StringArray("filter", []string{}, "Filter by field (format: field=value or field:value, can be used multiple times)")
	cmd.Flags().String("sort-by", "", "Field to sort by")
	cmd.Flags().Bool("sort-asc", true, "Sort ascending (default: true)")
	cmd.Flags().Int("offset", 0, "Pagination offset")
	cmd.Flags().Int("limit", 0, "Pagination limit (0 = no limit)")
	cmd.Flags().String("group-by", "", "Field to group results by")
	cmd.Flags().Int("group-limit", 0, "Maximum items per group when using --group-by (0 = no limit)")
	cmd.Flags().Bool("count", false, "Only return the count of matching objects (no object details)")
	cmd.Flags().StringArray("fields", nil, "Keys to include per object (repeat or comma-separated); omit for full rows. Sort/group columns merge automatically. Put -h/--help before --fields so a value like '-h' is not parsed as a flag.")
}

// AddCountFlags adds flags specific to count operations
func AddCountFlags(cmd *cobra.Command) {
	cmd.Flags().StringArray("filter", []string{}, "Filter by field (format: field=value or field:value, can be used multiple times)")
	cmd.Flags().String("group-by", "", "Field to group counts by (returns count per group)")
	cmd.Flags().Bool("include-zero-count", false, "Include kinds with 0 objects (default omits them to reduce noise)")
}

// CountHarnessFlagNames returns flag names registered by AddCountFlags.
func CountHarnessFlagNames() []string {
	return []string{"filter", "group-by", "include-zero-count"}
}

// ParseCountFlags parses flags for count operations (simpler than full query flags)
// logger can be nil if error logging is not needed
func ParseCountFlags(cmd *cobra.Command, logger Logger, parseFilter func(string) (string, any, error)) (map[string]any, string, error) {
	filters := make(map[string]any)
	filterStrs, err := cmd.Flags().GetStringArray("filter")
	if err != nil {
		filterStrs = []string{}
	}
	for _, filterStr := range filterStrs {
		fieldName, filterValue, err := parseFilter(filterStr)
		if err != nil {
			if logger != nil {
				if r, ok := logging.TryFluentEvent(logger); ok {
					r.Error("Failed to parse filter", err).FilterExpr(filterStr).Log()
				} else {
					logger.LogError("Failed to parse filter", err, logging.String("filter", filterStr))
				}
			}
			return nil, "", err
		}
		filters[fieldName] = filterValue
	}

	groupBy, _ := cmd.Flags().GetString("group-by")
	return filters, groupBy, nil
}
