package object

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/when"
	"github.com/lanceman/zqk/pkg/zqkenv"
	"github.com/spf13/cobra"

	"github.com/lanceman/zqk/pkg/objects"
)

func buildListOutputPayload(result *storage.QueryResult) map[string]any {
	output := make(map[string]any)
	when.When(func() bool { return len(result.Groups) > 0 }).Then(func() {
		output["groups"] = result.Groups
		totalGroups, ok := result.Meta["total_groups"].(int)
		when.When(func() bool { return ok }).Then(func() {
			output["total_groups"] = totalGroups
		}).Run()
	}).OrElse(func() {
		output["objects"] = result.Objects
	}).Run()
	if result.Meta != nil {
		output["meta"] = result.Meta
	}
	return output
}

// outputListStructured writes list results using the command's format (json/yaml via FormatOutput).
func outputListStructured(cmd *cobra.Command, result *storage.QueryResult) error {
	out := buildListOutputPayload(result)
	if err := cli.FormatOutput(cmd, out); err != nil {
		ctx := cli.GetContext(cmd)
		profile := string(pkgctx.ProfileSystem)
		if ctx != nil && ctx.Profile != emptyValue {
			profile = ctx.Profile
		}
		emitObjectListOutputErrorViaCoordinator(
			pkgctx.NewSystemContext(),
			"",  // projectRoot not available in output functions
			nil, // storageProvider not available in output functions
			string(cli.GetFormat(cmd)),
			err,
			profile,
		)
		return err
	}
	return nil
}

//nolint:gocyclo // Function orchestrates table output; complexity reduced via helper functions
func outputListTable(cmd *cobra.Command, result *storage.QueryResult, kind, groupBy string, tableFieldOrder []string) error {
	var buf strings.Builder

	// Load spec for this kind to get display_length defaults
	spec := loadSpecForKind(kind)

	when.When(func() bool { return len(result.Groups) > 0 }).Then(func() {
		outputGroupedTable(&buf, cmd, result, kind, groupBy, spec, tableFieldOrder)
	}).OrElse(func() {
		outputUngroupedTable(&buf, cmd, result, kind, spec, tableFieldOrder)
	}).Run()

	return cli.WriteOutput(cmd, []byte(buf.String()))
}

// loadSpecForKind loads the spec for a kind
func loadSpecForKind(kind string) *objects.Spec {
	if kind == emptyValue {
		return nil
	}
	specLoader := objects.GetGlobalSpecLoader()
	if loadedSpec, err := specLoader.LoadSpecWithInheritance(kind + ".yaml"); err == nil {
		return loadedSpec
	}
	return nil
}

// outputGroupedTable outputs a table for grouped results.
// POLICY-CODE-007: we build into buf then write to cmd stream via WriteString(Sprintf).
//
//nolint:gocritic // preferFprint
func outputGroupedTable(buf *strings.Builder, cmd *cobra.Command, result *storage.QueryResult, kind, groupBy string, spec *objects.Spec, tableFieldOrder []string) {
	when.When(func() bool { return kind != emptyValue }).Then(func() {
		if groupBy != "" {
			buf.WriteString(color.New(color.FgCyan, color.Bold).Sprintf("Grouped by %s for %s:\n", groupBy, kind))
		} else {
			buf.WriteString(color.New(color.FgCyan, color.Bold).Sprintf("Grouped results for %s:\n", kind))
		}
	}).OrElse(func() {
		if groupBy != "" {
			buf.WriteString(color.New(color.FgCyan, color.Bold).Sprintf("Grouped by %s:\n", groupBy))
		} else {
			buf.WriteString(color.New(color.FgCyan, color.Bold).Sprintf("Grouped results:\n"))
		}
	}).Run()
	if totalGroups, ok := result.Meta["total_groups"].(int); ok {
		buf.WriteString(color.New(color.FgHiBlack).Sprintf("Total groups: %d\n\n", totalGroups))
	}

	keys := make([]string, 0, len(result.Groups))
	for k := range result.Groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	maxRows := getMaxTableRows()
	for _, groupKey := range keys {
		objects := result.Groups[groupKey]
		if result.Meta == nil {
			result.Meta = make(map[string]any)
		}
		result.Meta["group_"+groupKey+"_total"] = len(objects)

		objectsToShow := objects
		if maxRows > 0 && len(objects) > maxRows {
			objectsToShow = objects[:maxRows]
		}

		outputGroupHeader(buf, result, groupKey, len(objectsToShow))
		if len(objectsToShow) > 0 {
			outputTableForObjects(buf, cmd, objectsToShow, spec, tableFieldOrder)
		}
		outputGroupFooter(buf, result, groupKey, len(objectsToShow))
	}
}

// defaultMaxTableRows caps table output to avoid very slow terminal rendering (e.g. 772 rows ~300s).
// Override with ZQK_TABLE_MAX_ROWS (0 = no cap). Full data via --format json.
const defaultMaxTableRows = 100

func getMaxTableRows() int {
	if v := os.Getenv(zqkenv.TableMaxRows()); v != emptyValue {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n
		}
	}
	return defaultMaxTableRows
}

// outputUngroupedTable outputs a table for ungrouped results.
//
//nolint:gocritic // preferFprint
func outputUngroupedTable(buf *strings.Builder, cmd *cobra.Command, result *storage.QueryResult, kind string, spec *objects.Spec, tableFieldOrder []string) {
	buf.WriteString(color.New(color.FgCyan, color.Bold).Sprintf("Objects of kind %s:\n", kind))
	totalCount := 0
	if result.Meta != nil {
		if total, ok := result.Meta["total_count"].(int); ok {
			totalCount = total
			buf.WriteString(color.New(color.FgHiBlack).Sprintf("Total: %d\n", total))
		}
	}
	buf.WriteString("\n")

	if len(result.Objects) > 0 {
		maxRows := getMaxTableRows()
		objectsToShow := result.Objects
		if maxRows > 0 && len(result.Objects) > maxRows {
			objectsToShow = result.Objects[:maxRows]
		}
		outputTableForObjects(buf, cmd, objectsToShow, spec, tableFieldOrder)
		if maxRows > 0 && len(result.Objects) > maxRows {
			totalForNote := totalCount
			if totalForNote <= 0 {
				totalForNote = len(result.Objects)
			}
			fmt.Fprintf(buf, "\n(showing first %d of %d; use --format json for full export)\n", maxRows, totalForNote)
		}
	}
}

// outputGroupHeader outputs the header for a group.
//
//nolint:gocritic // preferFprint
func outputGroupHeader(buf *strings.Builder, result *storage.QueryResult, groupKey string, shownCount int) {
	totalInGroup := shownCount
	if groupMeta, ok := result.Meta["group_"+groupKey+"_total"].(int); ok {
		totalInGroup = groupMeta
	}

	header := color.New(color.FgCyan, color.Bold).Sprintf("Group: %s", groupKey)
	when.When(func() bool { return totalInGroup > shownCount }).Then(func() {
		buf.WriteString(fmt.Sprintf("%s %s\n", header, color.New(color.FgHiBlack).Sprintf("(%d shown of %d total)", shownCount, totalInGroup)))
	}).OrElse(func() {
		buf.WriteString(fmt.Sprintf("%s %s\n", header, color.New(color.FgHiBlack).Sprintf("(%d items)", shownCount)))
	}).Run()
}

// outputGroupFooter outputs the footer for a group.
//
//nolint:gocritic // preferFprint
func outputGroupFooter(buf *strings.Builder, result *storage.QueryResult, groupKey string, shownCount int) {
	totalInGroup := shownCount
	if groupMeta, ok := result.Meta["group_"+groupKey+"_total"].(int); ok {
		totalInGroup = groupMeta
	}

	if totalInGroup > shownCount {
		fmt.Fprintf(buf, "  ... (%d more)\n", totalInGroup-shownCount)
	}
	buf.WriteString("\n")
}

// outputTableForObjects outputs a table for a list of objects
func outputTableForObjects(buf *strings.Builder, cmd *cobra.Command, objectList []map[string]any, spec *objects.Spec, tableFieldOrder []string) {
	columnFields, columnWidths, columnHeaders := parseTableColumns(cmd, spec, tableFieldOrder)

	var rows [][]string

	// Prepare rows
	for _, obj := range objectList {
		// Add display fields for table context (preserves originals, adds d_* fields)
		clipkg.AddDisplayFieldsForTable(obj, spec, cmd)

		values := make([]string, len(columnFields))
		for i, field := range columnFields {
			// Use display value (prefers d_* fields for table views)
			values[i] = clipkg.GetDisplayValue(obj, field, false)
		}
		rows = append(rows, values)
	}

	// Render table using the new table formatter
	buf.WriteString(clipkg.RenderTable(columnHeaders, columnWidths, rows))
	buf.WriteString("\n")
}

// parseTableColumns parses the --columns flag and returns column configuration.
// When tableFieldOrder is non-empty (--fields), column set and order follow projection; --columns only
// supplies per-field widths (via [clipkg.GetColumnWidth] inside projectionTableColumns), so e.g.
// `--fields id,status,priority_tier --columns id:36,priority_tier:15` does not drop status.
// Without projection, a non-empty --columns still replaces the column list (legacy parseColumnsFromFlag).
func parseTableColumns(cmd *cobra.Command, spec *objects.Spec, tableFieldOrder []string) (fields []string, widths []int, headers []string) {
	columnsStr, _ := cmd.Flags().GetString("columns") //nolint:errcheck // Flag parsing errors are non-critical

	if len(tableFieldOrder) > 0 {
		return projectionTableColumns(cmd, spec, tableFieldOrder)
	}

	if columnsStr != emptyValue {
		return parseColumnsFromFlag(columnsStr)
	}

	return getDefaultColumns(cmd, spec)
}

// projectionTableColumns builds column layout for table output matching list field projection.
func projectionTableColumns(cmd *cobra.Command, spec *objects.Spec, fieldOrder []string) (fields []string, widths []int, headers []string) {
	const defaultWidth = 24

	customWidths := make(map[string]int)
	if raw, err := cmd.Flags().GetStringArray("fields"); err == nil {
		for _, s := range raw {
			for _, part := range strings.Split(s, ",") {
				part = strings.TrimSpace(part)
				if fieldPart, widthPart, ok := strings.Cut(part, ":"); ok {
					fieldName := strings.TrimSpace(fieldPart)
					if w, err := strconv.Atoi(strings.TrimSpace(widthPart)); err == nil && w > 0 {
						customWidths[fieldName] = w
					}
				}
			}
		}
	}

	for _, f := range fieldOrder {
		fields = append(fields, f)
		headers = append(headers, formatColumnHeader(f))
		dw := defaultWidth
		switch f {
		case "id":
			dw = 15
		case "title", "name":
			dw = 50
		case "status":
			dw = 10
		}
		if w, ok := customWidths[f]; ok {
			widths = append(widths, w)
		} else {
			widths = append(widths, clipkg.GetColumnWidth(f, cmd, spec, dw))
		}
	}
	return fields, widths, headers
}

// parseColumnsFromFlag parses columns from the --columns flag
func parseColumnsFromFlag(columnsStr string) (fields []string, widths []int, headers []string) {
	var columnFields []string
	var columnWidths []int
	var columnHeaders []string

	for part := range strings.SplitSeq(columnsStr, ",") {
		part = strings.TrimSpace(part)
		if part == emptyValue {
			continue
		}

		// Split on colon
		kv := strings.SplitN(part, ":", 2)
		if len(kv) != 2 {
			continue // Skip invalid format
		}

		field := strings.TrimSpace(kv[0])
		widthStr := strings.TrimSpace(kv[1])
		width, err := strconv.Atoi(widthStr)
		if err != nil || width <= 0 {
			continue // Skip invalid width
		}

		columnFields = append(columnFields, field)
		columnWidths = append(columnWidths, width)
		columnHeaders = append(columnHeaders, formatColumnHeader(field))
	}

	return columnFields, columnWidths, columnHeaders
}

// formatColumnHeader formats a field name as a column header
func formatColumnHeader(field string) string {
	if field == emptyValue {
		return ""
	}
	header := strings.ToUpper(field[:1]) + field[1:]
	// Handle snake_case
	header = strings.ReplaceAll(header, "_", " ")
	return header
}

// getDefaultColumns returns default column configuration
func getDefaultColumns(cmd *cobra.Command, spec *objects.Spec) (fields []string, widths []int, headers []string) {
	columnFields := []string{"id", "title", "status"}
	columnHeaders := []string{"ID", "Title", "Status"}
	columnWidths := []int{
		clipkg.GetColumnWidth("id", cmd, spec, 15),
		clipkg.GetColumnWidth("title", cmd, spec, 50),
		clipkg.GetColumnWidth("status", cmd, spec, 10),
	}
	return columnFields, columnWidths, columnHeaders
}

// outputListTableGroupedByKind outputs a table grouped by kind (for list with no kind argument).
//
//nolint:gocritic // preferFprint
func outputListTableGroupedByKind(cmd *cobra.Command, result *storage.QueryResult, tableFieldOrder []string) error {
	var buf strings.Builder

	if len(result.Groups) == 0 {
		buf.WriteString("No objects found.\n")
		return cli.WriteOutput(cmd, []byte(buf.String()))
	}

	buf.WriteString("Objects grouped by kind:\n")
	if totalGroups, ok := result.Meta["total_groups"].(int); ok {
		fmt.Fprintf(&buf, "Total kinds: %d\n", totalGroups)
	}
	if totalCount, ok := result.Meta["total_count"].(int); ok {
		fmt.Fprintf(&buf, "Total objects: %d\n", totalCount)
	}
	buf.WriteString("\n")

	// Sort kinds alphabetically for consistent output
	type kindGroup struct {
		kind    string
		objects []map[string]any
	}
	sorted := make([]kindGroup, 0, len(result.Groups))
	for kind, objects := range result.Groups {
		sorted = append(sorted, kindGroup{kind: kind, objects: objects})
	}

	// Simple sort by kind name
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i].kind > sorted[j].kind {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	maxRows := getMaxTableRows()
	for _, kg := range sorted {
		objectsToShow := kg.objects
		if maxRows > 0 && len(kg.objects) > maxRows {
			objectsToShow = kg.objects[:maxRows]
		}

		fmt.Fprintf(&buf, "Kind: %s (%d items)\n", kg.kind, len(kg.objects))

		// Load spec for this kind to get display_length defaults
		var spec *objects.Spec
		specLoader := objects.GetGlobalSpecLoader()
		if loadedSpec, err := specLoader.LoadSpecWithInheritance(kg.kind + ".yaml"); err == nil {
			spec = loadedSpec
		}

		if len(objectsToShow) > 0 {
			outputTableForObjects(&buf, cmd, objectsToShow, spec, tableFieldOrder)
		}
		if maxRows > 0 && len(kg.objects) > maxRows {
			fmt.Fprintf(&buf, "\n(showing first %d of %d; use --format json for full export)\n", maxRows, len(kg.objects))
		}
		buf.WriteString("\n")
	}

	return cli.WriteOutput(cmd, []byte(buf.String()))
}
