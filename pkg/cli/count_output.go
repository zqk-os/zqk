package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"gopkg.in/yaml.v3"
)

// countOutKey* are JSON/YAML keys for count command output payloads.
const (
	countOutKeyCount         = "count"
	countOutKeyCountScope    = "count_scope"
	countOutKeyCountsByGroup = "counts_by_group"
	countOutKeyCountsByKind  = "counts_by_kind"
	countOutKeyMeta          = "meta"
	countOutKeyReturned      = "returned"
	countOutKeyReturnedCount = "returned_count"
	countOutKeyTotalCount    = "total_count"
	countOutKeyTotalGroups   = "total_groups"
	countOutKeyTotalKinds    = "total_kinds"
	countOutKeyTotalObjects  = "total_objects"
)

// CountResult represents count query results without importing pkg/storage
// This breaks the import cycle between pkg/cli and pkg/storage
type CountResult struct {
	Objects []map[string]any
	Groups  map[string][]map[string]any
	Meta    map[string]any
}

// OutputCount formats count results in the requested format and returns the formatted bytes
// Callers should use internal/cli.WriteOutput to write the result
// This is a shared utility for count operations across object, internal, and other commands
func OutputCount(result *CountResult, format, kind, groupBy string) ([]byte, error) {
	switch format {
	case OutputFormatJSON:
		return OutputCountJSON(result, groupBy)
	case OutputFormatYAML:
		return OutputCountYAML(result, groupBy)
	case OutputFormatTable:
		return OutputCountTable(result, kind, groupBy)
	default:
		// Default to table for unknown formats
		return OutputCountTable(result, kind, groupBy)
	}
}

// OutputCountJSON formats count results in JSON format
func OutputCountJSON(result *CountResult, groupBy string) ([]byte, error) {
	output := make(map[string]any)

	if groupBy != emptyValue && len(result.Groups) > 0 {
		// Grouped counts
		counts := make(map[string]int)
		for groupKey, objects := range result.Groups {
			counts[groupKey] = len(objects)
		}
		output[countOutKeyCountsByGroup] = counts
		if totalGroups, ok := result.Meta[countOutKeyTotalGroups].(int); ok {
			output[countOutKeyTotalGroups] = totalGroups
		}
	} else {
		// Total count
		if total, ok := result.Meta[countOutKeyTotalCount].(int); ok {
			output[countOutKeyCount] = total
		} else {
			output[countOutKeyCount] = len(result.Objects)
		}
		if returned, ok := result.Meta[countOutKeyReturnedCount].(int); ok {
			output[countOutKeyReturned] = returned
		}
	}

	if result.Meta != nil {
		output[countOutKeyMeta] = result.Meta
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return nil, errfmt.Newf("error marshaling JSON").Wrap(err)
	}
	return data, nil
}

// OutputCountYAML formats count results in YAML format
func OutputCountYAML(result *CountResult, groupBy string) ([]byte, error) {
	output := make(map[string]any)

	if groupBy != emptyValue && len(result.Groups) > 0 {
		// Grouped counts
		counts := make(map[string]int)
		for groupKey, objects := range result.Groups {
			counts[groupKey] = len(objects)
		}
		output[countOutKeyCountsByGroup] = counts
		if totalGroups, ok := result.Meta[countOutKeyTotalGroups].(int); ok {
			output[countOutKeyTotalGroups] = totalGroups
		}
	} else {
		// Total count
		if total, ok := result.Meta[countOutKeyTotalCount].(int); ok {
			output[countOutKeyCount] = total
		} else {
			output[countOutKeyCount] = len(result.Objects)
		}
		if returned, ok := result.Meta[countOutKeyReturnedCount].(int); ok {
			output[countOutKeyReturned] = returned
		}
	}

	if result.Meta != nil {
		output[countOutKeyMeta] = result.Meta
	}

	data, err := yaml.Marshal(output)
	if err != nil {
		return nil, errfmt.Newf("error marshaling YAML").Wrap(err)
	}
	return data, nil
}

// OutputCountTable formats count results in table format
func OutputCountTable(result *CountResult, kind, groupBy string) ([]byte, error) {
	var buf strings.Builder

	if groupBy != emptyValue && len(result.Groups) > 0 {
		// Grouped counts
		headerName := strings.ToUpper(groupBy)
		if headerName == "" {
			headerName = "GROUP"
		}
		headers := []string{headerName, "COUNT"}
		var rows [][]string

		keys := make([]string, 0, len(result.Groups))
		for k := range result.Groups {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, groupKey := range keys {
			groupDisplay := groupKey
			if groupDisplay == emptyValue {
				groupDisplay = "(empty)"
			}
			rows = append(rows, []string{groupDisplay, fmt.Sprintf("%d", len(result.Groups[groupKey]))})
		}

		title := fmt.Sprintf("Counts for %s grouped by %s", kind, groupBy)
		if kind == "" {
			title = fmt.Sprintf("Counts grouped by %s", groupBy)
		}

		buf.WriteString(RenderTableWithTitle(title, headers, nil, rows))
		if totalGroups, ok := result.Meta[countOutKeyTotalGroups].(int); ok {
			fmt.Fprintf(&buf, "\nTotal groups: %d\n", totalGroups)
		}
	} else {
		// Total count
		headers := []string{"KIND", "COUNT"}
		var count int
		if total, ok := result.Meta[countOutKeyTotalCount].(int); ok {
			count = total
		} else {
			count = len(result.Objects)
		}
		kindDisplay := kind
		if kindDisplay == "" {
			kindDisplay = "all"
		}
		rows := [][]string{{kindDisplay, fmt.Sprintf("%d", count)}}
		buf.WriteString(RenderTable(headers, nil, rows))
		if line := namespaceLineFromMeta(result.Meta); line != "" {
			fmt.Fprintf(&buf, "\n%s\n", line)
		}
	}
	return []byte(buf.String()), nil
}

// OutputCountDirect formats a direct count value (from Count() method, no grouping).
// Optional meta (e.g. namespace_scope) is included in JSON/YAML and summarized in table output.
func OutputCountDirect(count int, format, kind string, meta map[string]any) ([]byte, error) {
	switch format {
	case OutputFormatJSON:
		output := map[string]any{
			countOutKeyCount:     count,
			objects.FieldKeyKind: kind,
		}
		if len(meta) > 0 {
			output[countOutKeyMeta] = meta
		}
		data, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			return nil, errfmt.Newf("error marshaling JSON").Wrap(err)
		}
		return data, nil
	case OutputFormatYAML:
		output := map[string]any{
			countOutKeyCount:     count,
			objects.FieldKeyKind: kind,
		}
		if len(meta) > 0 {
			output[countOutKeyMeta] = meta
		}
		data, err := yaml.Marshal(output)
		if err != nil {
			return nil, errfmt.Newf("error marshaling YAML").Wrap(err)
		}
		return data, nil
	default:
		var buf strings.Builder
		if line := namespaceLineFromMeta(meta); line != "" {
			fmt.Fprintf(&buf, "%s\n", line)
		}
		fmt.Fprintf(&buf, "%d\n", count)
		return []byte(buf.String()), nil
	}
}

// OutputAllKindsCount formats counts for all kinds.
// scopeNote is optional: when non-empty, table output appends a line clarifying what is counted (e.g. "internal/built-in only" vs "all objects in system").
// meta is optional inventory scope (namespace_scope, …).
func OutputAllKindsCount(counts map[string]int, format string, scopeNote string, meta map[string]any) ([]byte, error) {
	switch format {
	case OutputFormatJSON:
		return OutputAllKindsCountJSON(counts, scopeNote, meta)
	case OutputFormatYAML:
		return OutputAllKindsCountYAML(counts, scopeNote, meta)
	default:
		return OutputAllKindsCountTable(counts, scopeNote, meta)
	}
}

// OutputAllKindsCountJSON formats counts for all kinds in JSON format
func OutputAllKindsCountJSON(counts map[string]int, scopeNote string, meta map[string]any) ([]byte, error) {
	output := map[string]any{
		countOutKeyCountsByKind: counts,
		countOutKeyTotalKinds:   len(counts),
	}

	totalCount := 0
	for _, count := range counts {
		totalCount += count
	}
	output[countOutKeyTotalObjects] = totalCount
	if scopeNote != emptyValue {
		output[countOutKeyCountScope] = scopeNote
	}
	if len(meta) > 0 {
		output[countOutKeyMeta] = meta
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return nil, errfmt.Newf("error marshaling JSON").Wrap(err)
	}
	return data, nil
}

// OutputAllKindsCountYAML formats counts for all kinds in YAML format
func OutputAllKindsCountYAML(counts map[string]int, scopeNote string, meta map[string]any) ([]byte, error) {
	output := map[string]any{
		countOutKeyCountsByKind: counts,
		countOutKeyTotalKinds:   len(counts),
	}

	totalCount := 0
	for _, count := range counts {
		totalCount += count
	}
	output[countOutKeyTotalObjects] = totalCount
	if scopeNote != emptyValue {
		output[countOutKeyCountScope] = scopeNote
	}
	if len(meta) > 0 {
		output[countOutKeyMeta] = meta
	}

	data, err := yaml.Marshal(output)
	if err != nil {
		return nil, errfmt.Newf("error marshaling YAML").Wrap(err)
	}
	return data, nil
}

// OutputAllKindsCountTable formats counts for all kinds in table format.
// When scopeNote is non-empty, appends a line clarifying what is counted (avoids confusion between internal count vs object count).
func OutputAllKindsCountTable(counts map[string]int, scopeNote string, meta map[string]any) ([]byte, error) {
	var buf strings.Builder

	buf.WriteString("Counts for all object kinds:\n\n")
	if line := namespaceLineFromMeta(meta); line != "" {
		fmt.Fprintf(&buf, "%s\n\n", line)
	}

	// Sort by count (descending) then by kind name
	type kindCount struct {
		kind  string
		count int
	}
	sorted := make([]kindCount, 0, len(counts))
	for kind, count := range counts {
		sorted = append(sorted, kindCount{kind: kind, count: count})
	}

	// Simple sort: by count descending, then by kind ascending
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].count != sorted[j].count {
			return sorted[i].count > sorted[j].count
		}
		return sorted[i].kind < sorted[j].kind
	})

	totalCount := 0
	for _, kc := range sorted {
		fmt.Fprintf(&buf, "  %s: %d\n", kc.kind, kc.count)
		totalCount += kc.count
	}

	fmt.Fprintf(&buf, "\nTotal kinds: %d\n", len(counts))
	fmt.Fprintf(&buf, "Total objects: %d\n", totalCount)
	if scopeNote != emptyValue {
		fmt.Fprintf(&buf, "(%s)\n", scopeNote)
	}
	return []byte(buf.String()), nil
}

func namespaceLineFromMeta(meta map[string]any) string {
	if meta == nil {
		return ""
	}
	mode, _ := meta["namespace_scope_mode"].(string)
	if mode == "" {
		return ""
	}
	if mode == "federated" {
		return "Namespace: (federated — all namespaces)"
	}
	ns, _ := meta["namespace_scope"].(string)
	if ns == "" {
		return ""
	}
	line := "Namespace: " + ns
	if h, ok := meta["hidden_outside_scope"].(int); ok && h > 0 {
		line += fmt.Sprintf(" (hiding %d outside scope)", h)
	}
	return line
}
