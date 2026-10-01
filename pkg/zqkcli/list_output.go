package internal

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/storage"

	"github.com/zqk-os/zqk/pkg/objects"
)

const (
	internalColumnsFlagName = "columns"
	columnSeparator         = ","
	columnWidthSeparator    = ":"
	listPayloadKeyIDs       = "ids"
	listPayloadKeyGroups    = "groups"
	listPayloadKeyObjects   = "objects"
	listPayloadKeyMeta      = "meta"
	listMetaKeyTotalCount   = "total_count"
	listMetaKeyTotalGroups  = "total_groups"
)

var defaultTableColumnFields = []string{"id", "title", "status"}

var defaultTableColumnHeaders = []string{"ID", "Title", "Status"}

var defaultTableColumnFallbackWidths = []int{15, 50, 10}

func outputAllInternalTable(cmd *cobra.Command, allResults []map[string]any, kindCounts map[string]int, kindTruncated map[string]bool, builtInOnly, internalOnly, allObjects bool) {
	var filterDesc string
	if builtInOnly {
		filterDesc = internalSourceBuiltIn
	} else if internalOnly {
		filterDesc = internalSourceInternal
	} else if allObjects {
		filterDesc = "all"
	} else {
		filterDesc = internalSourceBuiltIn + " and " + internalSourceInternal
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "All %s objects:\n", filterDesc)
	total := 0
	for _, count := range kindCounts {
		total += count
	}
	fmt.Fprintf(&buf, "Total: %d\n\n", total)

	// Group by kind for display
	grouped := make(map[string][]map[string]any)
	for _, obj := range allResults {
		objKind, _ := obj[objects.FieldKeyKind].(string)
		if objKind == emptyValue {
			objKind = "unknown"
		}
		grouped[objKind] = append(grouped[objKind], obj)
	}

	// Output each kind with total counts
	// Sort kinds for consistent output
	kinds := make([]string, 0, len(grouped))
	for kind := range grouped {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)

	for _, kind := range kinds {
		kindObjects := grouped[kind]
		totalCount := kindCounts[kind]
		truncated := kindTruncated[kind]

		if len(kindObjects) > 0 {
			if truncated {
				fmt.Fprintf(&buf, "%s (%d shown of %d total):\n", kind, len(kindObjects), totalCount)
			} else {
				fmt.Fprintf(&buf, "%s (%d items):\n", kind, totalCount)
			}
			for _, obj := range kindObjects {
				outputObjectLine(&buf, obj)
			}
			if truncated {
				fmt.Fprintf(&buf, "  ... (%d more)\n", totalCount-len(kindObjects))
			}
			buf.WriteString("\n")
		} else if totalCount > 0 {
			// Show count even if no objects displayed (shouldn't happen, but just in case)
			fmt.Fprintf(&buf, "%s (%d items, none displayed)\n\n", kind, totalCount)
		}
	}
	//nolint:errcheck // Output errors are non-critical
	_ = cli.WriteOutput(cmd, buf.Bytes())
}

func outputList(cmd *cobra.Command, result *storage.QueryResult, format cli.OutputFormat, kind string, idsOnly bool) {
	if idsOnly {
		outputListIDsOnly(cmd, result, format, kind)
		return
	}
	switch format {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		outputListStructured(cmd, result, kind)
	default:
		outputListTable(cmd, result, kind)
	}
}

// outputListIDsOnly outputs only object IDs; respects format (json/yaml = structured, default = one per line).
func outputListIDsOnly(cmd *cobra.Command, result *storage.QueryResult, format cli.OutputFormat, kind string) {
	ids := make([]string, 0)

	if len(result.Groups) > 0 {
		for _, groupObjs := range result.Groups {
			for _, obj := range groupObjs {
				if id, ok := obj[objects.FieldKeyID].(string); ok && id != emptyValue {
					ids = append(ids, id)
				}
			}
		}
	} else {
		for _, obj := range result.Objects {
			if id, ok := obj[objects.FieldKeyID].(string); ok && id != emptyValue {
				ids = append(ids, id)
			}
		}
	}

	sort.Strings(ids)

	switch format {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		out := buildIDsPayload(ids, kind, result.Meta)
		if err := cli.FormatOutput(cmd, out); err != nil {
			emitInternalListOutputErrorViaCoordinator(
				pkgctx.NewSystemContext(),
				"",
				nil,
				string(format),
				err,
				internalProfileSystem,
			)
		}
	default:
		var buf bytes.Buffer
		for _, id := range ids {
			buf.WriteString(id + "\n")
		}
		//nolint:errcheck // Output errors are non-critical
		_ = cli.WriteOutput(cmd, buf.Bytes())
	}
}

func buildListOutputMap(result *storage.QueryResult, kind string) map[string]any {
	output := make(map[string]any)
	if len(result.Groups) > 0 {
		output[listPayloadKeyGroups] = result.Groups
		if totalGroups, ok := result.Meta[listMetaKeyTotalGroups].(int); ok {
			output[listMetaKeyTotalGroups] = totalGroups
		}
	} else {
		output[listPayloadKeyObjects] = result.Objects
	}
	if result.Meta != nil {
		output[listPayloadKeyMeta] = result.Meta
	}
	output[objects.FieldKeyKind] = kind
	return output
}

func outputListStructured(cmd *cobra.Command, result *storage.QueryResult, kind string) {
	payload := buildListOutputMap(result, kind)
	if err := cli.FormatOutput(cmd, payload); err != nil {
		emitInternalListOutputErrorViaCoordinator(
			pkgctx.NewSystemContext(),
			"",
			nil,
			string(cli.GetFormat(cmd)),
			err,
			internalProfileSystem,
		)
	}
}

func outputListTable(cmd *cobra.Command, result *storage.QueryResult, kind string) {
	var buf bytes.Buffer

	// Load spec for this kind to get display_length defaults
	var spec *objects.Spec
	if kind != emptyValue {
		specLoader := objects.GetGlobalSpecLoader()
		if loadedSpec, err := specLoader.LoadSpecWithInheritance(kind + ".yaml"); err == nil {
			spec = loadedSpec
		}
	}

	if len(result.Groups) > 0 {
		fmt.Fprintf(&buf, "Grouped results for %s:\n", kind)
		if totalGroups, ok := result.Meta["total_groups"].(int); ok {
			fmt.Fprintf(&buf, "Total groups: %d\n\n", totalGroups)
		}
		for groupKey, objects := range result.Groups {
			fmt.Fprintf(&buf, "Group: %s (%d items)\n", groupKey, len(objects))

			if len(objects) > 0 {
				columnFields, columnWidths, columnHeaders := resolveInternalTableColumns(cmd, spec)

				// Render table using RenderTable
				var rows [][]string
				for _, obj := range objects {
					clipkg.AddDisplayFieldsForTable(obj, spec, cmd)
					values := make([]string, len(columnFields))
					for i, field := range columnFields {
						values[i] = clipkg.GetDisplayValue(obj, field, false)
					}
					rows = append(rows, values)
				}
				buf.WriteString(clipkg.RenderTable(columnHeaders, columnWidths, rows))
				buf.WriteString("\n")
			}
			buf.WriteString("\n")
		}
	} else {
		fmt.Fprintf(&buf, "Objects of kind %s:\n", kind)
		if result.Meta != nil {
			if total, ok := result.Meta[listMetaKeyTotalCount].(int); ok {
				fmt.Fprintf(&buf, "Total: %d\n", total)
			}
		}
		buf.WriteString("\n")

		if len(result.Objects) > 0 {
			columnFields, columnWidths, columnHeaders := resolveInternalTableColumns(cmd, spec)

			// Render table using RenderTable
			var rows [][]string
			for _, obj := range result.Objects {
				clipkg.AddDisplayFieldsForTable(obj, spec, cmd)
				values := make([]string, len(columnFields))
				for i, field := range columnFields {
					values[i] = clipkg.GetDisplayValue(obj, field, false)
				}
				rows = append(rows, values)
			}
			buf.WriteString(clipkg.RenderTable(columnHeaders, columnWidths, rows))
			buf.WriteString("\n")
		}
	}
	//nolint:errcheck // Output errors are non-critical
	_ = cli.WriteOutput(cmd, buf.Bytes())
}

func resolveInternalTableColumns(cmd *cobra.Command, spec *objects.Spec) ([]string, []int, []string) {
	var flagsBag clipkg.FlagBag
	columnsStr := flagsBag.String(cmd, internalColumnsFlagName)
	columnFields, columnWidths, columnHeaders := parseInternalColumnsSpec(columnsStr)
	if len(columnFields) > 0 {
		return columnFields, columnWidths, columnHeaders
	}

	fallbackFields := append([]string(nil), defaultTableColumnFields...)
	fallbackHeaders := append([]string(nil), defaultTableColumnHeaders...)
	fallbackWidths := make([]int, len(defaultTableColumnFields))
	for i, field := range defaultTableColumnFields {
		fallbackWidths[i] = clipkg.GetColumnWidth(field, cmd, spec, defaultTableColumnFallbackWidths[i])
	}
	return fallbackFields, fallbackWidths, fallbackHeaders
}

func parseInternalColumnsSpec(columnsStr string) ([]string, []int, []string) {
	fields := make([]string, 0)
	widths := make([]int, 0)
	headers := make([]string, 0)

	for part := range strings.SplitSeq(columnsStr, columnSeparator) {
		part = strings.TrimSpace(part)
		if part == emptyValue {
			continue
		}

		fieldPart, widthPart, ok := strings.Cut(part, columnWidthSeparator)
		if !ok {
			continue
		}
		field := strings.TrimSpace(fieldPart)
		widthStr := strings.TrimSpace(widthPart)
		width, err := strconv.Atoi(widthStr)
		if err != nil || width <= 0 || field == emptyValue {
			continue
		}

		fields = append(fields, field)
		widths = append(widths, width)
		header := strings.ToUpper(field[:1]) + field[1:]
		headers = append(headers, strings.ReplaceAll(header, "_", " "))
	}

	return fields, widths, headers
}

func buildIDsPayload(ids []string, kind string, meta map[string]any) map[string]any {
	out := map[string]any{listPayloadKeyIDs: ids, objects.FieldKeyKind: kind}
	if meta != nil {
		if total, ok := meta[listMetaKeyTotalCount].(int); ok {
			out[listMetaKeyTotalCount] = total
		}
	}
	return out
}

// getStringValueInternal safely extracts a string value from a map

func outputObjectLine(buf *bytes.Buffer, obj map[string]any) {
	id, _ := obj[objects.FieldKeyID].(string)
	title := ""
	if t, ok := obj[objects.FieldKeyTitle].(string); ok {
		title = t
	}
	status := ""
	if s, ok := obj[objects.FieldKeyStatus].(string); ok {
		status = s
	}

	builtIn := storage.IsBuiltIn(obj)
	builtInMarker := ""
	if builtIn {
		builtInMarker = " [BUILT-IN]"
	}

	if status != emptyValue {
		//nolint:gocritic // preferFprint: Using WriteString with fmt.Sprintf to comply with POL-CODE-007
		fmt.Fprintf(buf, "  %s: %s [%s]%s\n", id, title, status, builtInMarker)
	} else {
		//nolint:gocritic // preferFprint: Using WriteString with fmt.Sprintf to comply with POL-CODE-007
		fmt.Fprintf(buf, "  %s: %s%s\n", id, title, builtInMarker)
	}
}
