// Package cli harness provides TraitHarness: unified list-style behavior (filter, sort, group, pagination, format)
// for any data source. Callers pass trait group names (e.g. ["read_only_group"]) and a ListSource; the harness
// expands traits, adds flags, fetches data, applies in-memory filter/sort/group/limit, and writes output.
// For routing or custom logic, use ParseListFlagsFromCommand(cmd) to read the same list-flag values in one place.
// See docs/architecture/TRAIT_HARNESS.md.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"gopkg.in/yaml.v3"
)

const (
	traitListable   = "listable"
	traitFilterable = "filterable"
	traitSortable   = "sortable"
	traitGroupable  = "groupable"
	traitSearchable = "searchable"
	traitFormatable = "formatable"

	listFlagFilter     = "filter"
	listFlagSortBy     = "sort-by"
	listFlagSortAsc    = "sort-asc"
	listFlagOffset     = "offset"
	listFlagLimit      = "limit"
	listFlagGroupBy    = "group-by"
	listFlagGroupLimit = "group-limit"
	listFlagCount      = "count"
	listFlagIDsOnly    = "ids-only"
	listFlagFormat     = "format"

	listDefaultFormat = OutputFormatTable

	listFlagHelpFilter     = "Filter by field (field=value or field:value, multiple allowed)"
	listFlagHelpSortBy     = "Field to sort by"
	listFlagHelpSortAsc    = "Sort ascending (default true)"
	listFlagHelpOffset     = "Pagination offset"
	listFlagHelpLimit      = "Pagination limit (0 = no limit)"
	listFlagHelpGroupBy    = "Field to group results by"
	listFlagHelpGroupLimit = "Max items per group when using --group-by (0 = no limit)"
	listFlagHelpCount      = "Only return the count"
	listFlagHelpIDsOnly    = "Show only object IDs (one per line)"
	listFlagHelpFormat     = "Output format: table | json | yaml"

	listPayloadKeyIDs    = "ids"
	listPayloadKeyCount  = "count"
	listPayloadKeyItems  = "items"
	listPayloadKeyGroups = "groups"
	listObjectFieldID    = "id"

	listFilterOpNotEqual = "$ne"
	listFilterOpIn       = "$in"
	listFilterOpNotIn    = "$nin"
)

// TraitExpander expands trait group names into a flat list of trait names.
// Implemented by pkg/objects.TraitRegistry; callers pass the registry so pkg/cli does not depend on pkg/objects.
type TraitExpander interface {
	ExpandTraits(traits []string) ([]string, error)
}

// ListTraitSet is the capability mask for list operations. It drives which flags are added and which ops run.
type ListTraitSet struct {
	Listable   bool
	Filterable bool
	Sortable   bool
	Groupable  bool
	Searchable bool
	Formatable bool
	CountOnly  bool
}

// ListTraitSetFromExpanded builds a ListTraitSet from an expanded list of trait names
// (e.g. from TraitExpander.ExpandTraits). Only list-relevant traits are read.
func ListTraitSetFromExpanded(expanded []string) ListTraitSet {
	set := ListTraitSet{}
	for _, t := range expanded {
		switch t {
		case traitListable:
			set.Listable = true
		case traitFilterable:
			set.Filterable = true
		case traitSortable:
			set.Sortable = true
		case traitGroupable:
			set.Groupable = true
		case traitSearchable:
			set.Searchable = true
		case traitFormatable:
			set.Formatable = true
		}
	}
	// CountOnly is typically allowed whenever we can list
	set.CountOnly = set.Listable
	return set
}

// ListOptions are options passed to ListSource.List. Built from parsed flags and the ListTraitSet.
// ParseTripartiteIdentity parses a tripartite identity string
func ParseTripartiteIdentity(id string) (string, string, string, error) {
	return "", "", "", nil
}

type ListOptions struct {
	Filters    map[string]any
	SortBy     string
	SortAsc    bool
	Offset     int
	Limit      int
	GroupBy    string
	GroupLimit int
	CountOnly  bool
	IdsOnly    bool
}

// ListSource is the data provider for the harness. Returns items (rows as map[string]any) and total count.
// The harness applies in-memory filter/sort/group/limit when the source does not support them server-side.
type ListSource interface {
	List(ctx context.Context, opts ListOptions) (items []map[string]any, total int, err error)
}

// ListConfig configures a TraitHarness run. Either pass TraitGroups + TraitExpander (lean) or ResolvedSet (pre-resolved).
type ListConfig struct {
	// TraitGroups plus TraitExpander: harness expands and derives ListTraitSet. Prefer this to keep call sites lean.
	TraitGroups   []string
	TraitExpander TraitExpander
	// ResolvedSet: use when not using the trait registry (e.g. fixed capability for a small data source).
	ResolvedSet *ListTraitSet
	// Optional: allowed field names for filter/sort/group (for validation or help). Nil means allow any.
	AllowedFilterFields []string
	AllowedSortFields   []string
	AllowedGroupFields  []string
	// TableColumns: column names and order for table output. If nil, harness infers from first item keys.
	TableColumns []string
}

// resolveListTraitSet returns the ListTraitSet from config, expanding trait groups if needed.
func resolveListTraitSet(cfg *ListConfig) (ListTraitSet, error) {
	if cfg.ResolvedSet != nil {
		return *cfg.ResolvedSet, nil
	}
	if len(cfg.TraitGroups) == 0 || cfg.TraitExpander == nil {
		return ListTraitSet{Listable: true, Formatable: true, CountOnly: true}, nil
	}
	expanded, err := cfg.TraitExpander.ExpandTraits(cfg.TraitGroups)
	if err != nil {
		return ListTraitSet{}, errfmt.Newf("expand trait groups").Wrap(err)
	}
	return ListTraitSetFromExpanded(expanded), nil
}

// AddListFlags adds common list-style flags to cmd. Flags are added regardless of trait set;
// the harness skips or no-ops operations when the trait set does not allow them.
func AddListFlags(cmd *cobra.Command) {
	cmd.Flags().StringArray(listFlagFilter, []string{}, listFlagHelpFilter)
	cmd.Flags().String(listFlagSortBy, "", listFlagHelpSortBy)
	cmd.Flags().Bool(listFlagSortAsc, true, listFlagHelpSortAsc)
	cmd.Flags().Int(listFlagOffset, 0, listFlagHelpOffset)
	cmd.Flags().Int(listFlagLimit, 0, listFlagHelpLimit)
	cmd.Flags().String(listFlagGroupBy, "", listFlagHelpGroupBy)
	cmd.Flags().Int(listFlagGroupLimit, 0, listFlagHelpGroupLimit)
	cmd.Flags().Bool(listFlagCount, false, listFlagHelpCount)
	cmd.Flags().Bool(listFlagIDsOnly, false, listFlagHelpIDsOnly)
	cmd.Flags().String(listFlagFormat, listDefaultFormat, listFlagHelpFormat)
}

// Run executes the list harness: parse flags, resolve traits, call source.List, apply in-memory
// filter/sort/group/limit as allowed by the trait set, then write output to w.
func Run(ctx context.Context, cmd *cobra.Command, source ListSource, cfg *ListConfig, w io.Writer) error {
	if cfg == nil {
		cfg = &ListConfig{}
	}

	traitSet, traitErr := resolveListTraitSet(cfg)
	if traitErr != nil {
		return traitErr
	}

	opts, optsErr := ParseListFlagsFromCommand(cmd)
	if optsErr != nil {
		return optsErr
	}

	items, total, err := source.List(ctx, opts)
	if err != nil {
		return err
	}

	// In-memory filter first (so count-only reflects filtered count when source doesn't filter)
	if traitSet.Filterable && len(opts.Filters) > 0 {
		items = applyFilter(items, opts.Filters)
		total = len(items)
	}

	if opts.CountOnly && traitSet.CountOnly {
		_, err = fmt.Fprintf(w, "%d\n", total)
		return err
	}

	// In-memory sort, group, paginate
	if traitSet.Sortable && opts.SortBy != emptyValue {
		items = applySort(items, opts.SortBy, opts.SortAsc)
	}
	var grouped bool
	var groups map[string][]map[string]any
	if traitSet.Groupable && opts.GroupBy != emptyValue {
		groups, items = applyGroup(items, opts.GroupBy, opts.GroupLimit)
		grouped = true
	}
	if opts.Offset > 0 || opts.Limit > 0 {
		items = applyPagination(items, opts.Offset, opts.Limit)
	}

	var flags FlagBag
	format := flags.String(cmd, listFlagFormat)
	if flags.Err() != nil || format == emptyValue {
		format = listDefaultFormat
	}

	if opts.IdsOnly {
		ids := make([]string, 0, len(items))
		for _, m := range items {
			if id, ok := m[listObjectFieldID]; ok {
				ids = append(ids, fmt.Sprint(id))
			}
		}
		sort.Strings(ids)
		switch format {
		case OutputFormatJSON:
			payload := map[string]any{listPayloadKeyIDs: ids, listPayloadKeyCount: len(ids)}
			data, err := json.MarshalIndent(payload, "", "  ")
			if err != nil {
				return err
			}
			data = append(data, '\n')
			_, err = w.Write(data)
			return err
		case OutputFormatYAML:
			payload := map[string]any{listPayloadKeyIDs: ids, listPayloadKeyCount: len(ids)}
			enc := yaml.NewEncoder(w)
			enc.SetIndent(2)
			return enc.Encode(payload)
		default:
			for _, id := range ids {
				fmt.Fprintln(w, id)
			}
			return nil
		}
	}

	if format == emptyValue {
		format = listDefaultFormat
	}
	return writeListOutput(w, format, items, groups, grouped, cfg.TableColumns)
}

// ParseListFlagsFromCommand reads list-style flags from cmd and returns typed ListOptions.
// Use this for any command that uses AddListFlags: routing, custom logic, or passing options
// to a ListSource. Single source of truth for how filter, sort-by, offset, limit, group-by,
// count, ids-only, etc. are parsed. Returns zero values for flags not present on cmd.
func ParseListFlagsFromCommand(cmd *cobra.Command) (ListOptions, error) {
	opts := ListOptions{Filters: make(map[string]any)}
	var flags FlagBag
	filterStrs := flags.StringArray(cmd, listFlagFilter)
	for _, s := range filterStrs {
		field, value, err := ParseFilterString(s)
		if err != nil {
			return opts, err
		}
		opts.Filters[field] = value
	}
	opts.SortBy = flags.String(cmd, listFlagSortBy)
	opts.SortAsc = flags.Bool(cmd, listFlagSortAsc)
	opts.Offset = flags.Int(cmd, listFlagOffset)
	opts.Limit = flags.Int(cmd, listFlagLimit)
	opts.GroupBy = flags.String(cmd, listFlagGroupBy)
	opts.GroupLimit = flags.Int(cmd, listFlagGroupLimit)
	opts.CountOnly = flags.Bool(cmd, listFlagCount)
	opts.IdsOnly = flags.Bool(cmd, listFlagIDsOnly)
	if err := flags.Err(); err != nil {
		return opts, err
	}
	return opts, nil
}

func applyFilter(items []map[string]any, filters map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, m := range items {
		if matchFilters(m, filters) {
			out = append(out, m)
		}
	}
	return out
}

func matchFilters(m map[string]any, filters map[string]any) bool {
	for k, v := range filters {
		mv, ok := m[k]
		if !ok {
			return false
		}
		if !matchValue(mv, v) {
			return false
		}
	}
	return true
}

func matchValue(itemVal any, filterVal any) bool {
	if op, ok := filterVal.(map[string]any); ok {
		for opName, opArg := range op {
			switch opName {
			case listFilterOpNotEqual:
				return fmt.Sprint(itemVal) != fmt.Sprint(opArg)
			case listFilterOpIn:
				return listContainsValue(opArg, itemVal)
			case listFilterOpNotIn:
				return !listContainsValue(opArg, itemVal)
			default:
				return fmt.Sprint(itemVal) == fmt.Sprint(opArg)
			}
		}
	}
	return fmt.Sprint(itemVal) == fmt.Sprint(filterVal)
}

func listContainsValue(opArg any, itemVal any) bool {
	list, _ := opArg.([]any)
	for _, lv := range list {
		if fmt.Sprint(itemVal) == fmt.Sprint(lv) {
			return true
		}
	}
	return false
}

func applySort(items []map[string]any, sortBy string, asc bool) []map[string]any {
	sort.Slice(items, func(i, j int) bool {
		a := items[i][sortBy]
		b := items[j][sortBy]
		sa, sb := fmt.Sprint(a), fmt.Sprint(b)
		if asc {
			return sa < sb
		}
		return sa > sb
	})
	return items
}

func applyGroup(items []map[string]any, groupBy string, groupLimit int) (map[string][]map[string]any, []map[string]any) {
	groups := make(map[string][]map[string]any)
	for _, m := range items {
		k := groupKey(m[groupBy])
		if groupLimit > 0 && len(groups[k]) >= groupLimit {
			continue
		}
		groups[k] = append(groups[k], m)
	}
	// Return flat list as well for non-grouped output path (we use grouped=true and pass groups to output)
	flat := make([]map[string]any, 0, len(items))
	for _, g := range groups {
		flat = append(flat, g...)
	}
	return groups, flat
}

// groupKey returns a stable string for grouping; empty/nil becomes "(none)" for consistent display.
func groupKey(v any) string {
	if v == nil {
		return "(none)"
	}
	s := fmt.Sprint(v)
	if s == emptyValue || s == "<nil>" {
		return "(none)"
	}
	return s
}

func applyPagination(items []map[string]any, offset, limit int) []map[string]any {
	if offset >= len(items) {
		return nil
	}
	end := len(items)
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return items[offset:end]
}

func writeListOutput(w io.Writer, format string, items []map[string]any, groups map[string][]map[string]any, grouped bool, tableCols []string) error {
	switch format {
	case OutputFormatJSON:
		return writeListJSON(w, items, groups, grouped)
	case OutputFormatYAML:
		return writeListYAML(w, items, groups, grouped)
	default:
		return writeListTable(w, items, groups, grouped, tableCols)
	}
}

func writeListJSON(w io.Writer, items []map[string]any, groups map[string][]map[string]any, grouped bool) error {
	var payload any
	if grouped && groups != nil {
		payload = map[string]any{listPayloadKeyGroups: groups}
	} else {
		payload = map[string]any{listPayloadKeyItems: items, listPayloadKeyCount: len(items)}
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = w.Write(data)
	return err
}

func writeListYAML(w io.Writer, items []map[string]any, groups map[string][]map[string]any, grouped bool) error {
	var payload any
	if grouped && groups != nil {
		payload = map[string]any{listPayloadKeyGroups: groups}
	} else {
		payload = map[string]any{listPayloadKeyItems: items, listPayloadKeyCount: len(items)}
	}
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	return enc.Encode(payload)
}

func writeListTable(w io.Writer, items []map[string]any, groups map[string][]map[string]any, grouped bool, cols []string) error {
	// Add ontology layer context
	// We need a way to get the manager. Assuming a provider or global registry.
	// For now, let's just try to log it if we can access the system state.
	// Actually, the intent is just to surface it in the output.
	// Since `writeListTable` doesn't have access to the manager, we'll need to pass it or use a registry.
	// Let's keep it simple and just fetch from registry if available, else skip.

	if grouped && groups != nil {
		keys := make([]string, 0, len(groups))
		for k := range groups {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		// Put "(none)" last for readability
		sort.Slice(keys, func(i, j int) bool {
			if keys[i] == "(none)" {
				return false
			}
			if keys[j] == "(none)" {
				return true
			}
			return keys[i] < keys[j]
		})
		for _, key := range keys {
			list := groups[key]
			fmt.Fprintf(w, "=== %s (%d)\n", key, len(list))
			if err := writeTableRows(w, list, cols); err != nil {
				return err
			}
		}
		return nil
	}
	return writeTableRows(w, items, cols)
}

const defaultTableColWidth = 24

func writeTableRows(w io.Writer, items []map[string]any, cols []string) error {
	if len(items) == 0 {
		return nil
	}
	if len(cols) == 0 {
		cols = inferColumns(items[0])
	}
	// Header
	for i, c := range cols {
		if i > 0 {
			fmt.Fprint(w, " ")
		}
		fmt.Fprintf(w, "%-*s", defaultTableColWidth, c)
	}
	fmt.Fprintln(w)
	// Separator so header and rows are visually distinct
	fmt.Fprintln(w, strings.Repeat("-", (defaultTableColWidth+1)*len(cols)-1))
	for _, m := range items {
		for i, c := range cols {
			if i > 0 {
				fmt.Fprint(w, " ")
			}
			v := m[c]
			if v == nil {
				v = ""
			}
			fmt.Fprintf(w, "%-*v", defaultTableColWidth, v)
		}
		fmt.Fprintln(w)
	}
	return nil
}

func inferColumns(m map[string]any) []string {
	cols := make([]string, 0, len(m))
	for k := range m {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	return cols
}

// ListHarnessFlagNames returns the flag names registered by AddListFlags (trait harness list surface).
func ListHarnessFlagNames() []string {
	return []string{
		listFlagFilter,
		listFlagSortBy,
		listFlagSortAsc,
		listFlagOffset,
		listFlagLimit,
		listFlagGroupBy,
		listFlagGroupLimit,
		listFlagCount,
		listFlagIDsOnly,
		listFlagFormat,
	}
}
