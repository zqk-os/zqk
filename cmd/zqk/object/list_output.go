package object

import (
	"io"

	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/config"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/when"
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
	out := cmd.OutOrStdout()

	// Load spec for this kind to get display_length defaults
	spec := loadSpecForKind(kind)

	when.When(func() bool { return len(result.Groups) > 0 }).Then(func() {
		outputGroupedTable(out, cmd, result, kind, groupBy, spec, tableFieldOrder)
	}).OrElse(func() {
		outputUngroupedTable(out, cmd, result, kind, spec, tableFieldOrder)
	}).Run()

	return nil
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
// POL-CODE-007: we build into buf then write to cmd stream via WriteString(Sprintf).
//
//nolint:gocritic // preferFprint
func outputGroupedTable(out io.Writer, cmd *cobra.Command, result *storage.QueryResult, kind, groupBy string, spec *objects.Spec, tableFieldOrder []string) {
	when.When(func() bool { return kind != emptyValue }).Then(func() {
		if groupBy != "" {
			fmt.Fprint(out, color.New(color.FgCyan, color.Bold).Sprintf("Grouped by %s for %s:\n", groupBy, kind))
		} else {
			fmt.Fprint(out, color.New(color.FgCyan, color.Bold).Sprintf("Grouped results for %s:\n", kind))
		}
	}).OrElse(func() {
		if groupBy != "" {
			fmt.Fprint(out, color.New(color.FgCyan, color.Bold).Sprintf("Grouped by %s:\n", groupBy))
		} else {
			fmt.Fprint(out, color.New(color.FgCyan, color.Bold).Sprintf("Grouped results:\n"))
		}
	}).Run()
	if totalGroups, ok := result.Meta["total_groups"].(int); ok {
		fmt.Fprint(out, color.New(color.FgHiBlack).Sprintf("Total groups: %d\n\n", totalGroups))
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

		outputGroupHeader(out, result, groupKey, len(objectsToShow))
		if len(objectsToShow) > 0 {
			outputTableForObjects(out, cmd, objectsToShow, spec, tableFieldOrder)
		}
		outputGroupFooter(out, result, groupKey, len(objectsToShow))
	}
}

// defaultMaxTableRows caps table output to avoid very slow terminal rendering (e.g. 772 rows ~300s).
// Override with ZQK_TABLE_MAX_ROWS (0 = no cap). Full data via --format json.
const defaultMaxTableRows = 100

func getMaxTableRows() int {
	return config.StorageTableMaxRows().OrDefault(100)
}

// outputUngroupedTable outputs a table for ungrouped results.
//
//nolint:gocritic // preferFprint
func outputUngroupedTable(out io.Writer, cmd *cobra.Command, result *storage.QueryResult, kind string, spec *objects.Spec, tableFieldOrder []string) {
	fmt.Fprint(out, color.New(color.FgCyan, color.Bold).Sprintf("Objects of kind %s:\n", kind))
	if scope, ok := namespaceScopeFromMeta(result.Meta); ok {
		fmt.Fprint(out, color.New(color.FgHiBlack).Sprintf("%s\n", formatNamespaceScopeTableLine(scope)))
	}
	totalCount := 0
	if result.Meta != nil {
		if total, ok := result.Meta["total_count"].(int); ok {
			totalCount = total
			fmt.Fprint(out, color.New(color.FgHiBlack).Sprintf("Total: %d\n", total))
		}
	}
	fmt.Fprint(out, "\n")

	if len(result.Objects) > 0 {
		maxRows := getMaxTableRows()
		objectsToShow := result.Objects
		if maxRows > 0 && len(result.Objects) > maxRows {
			objectsToShow = result.Objects[:maxRows]
		}
		outputTableForObjects(out, cmd, objectsToShow, spec, tableFieldOrder)
		if maxRows > 0 && len(result.Objects) > maxRows {
			totalForNote := totalCount
			if totalForNote <= 0 {
				totalForNote = len(result.Objects)
			}
			fmt.Fprintf(out, "\n(showing first %d of %d; use --format json for full export)\n", maxRows, totalForNote)
		}
	}
}

// outputGroupHeader outputs the header for a group.
//
//nolint:gocritic // preferFprint
func outputGroupHeader(out io.Writer, result *storage.QueryResult, groupKey string, shownCount int) {
	totalInGroup := shownCount
	if groupMeta, ok := result.Meta["group_"+groupKey+"_total"].(int); ok {
		totalInGroup = groupMeta
	}

	header := color.New(color.FgCyan, color.Bold).Sprintf("Group: %s", groupKey)
	when.When(func() bool { return totalInGroup > shownCount }).Then(func() {
		fmt.Fprint(out, fmt.Sprintf("%s %s\n", header, color.New(color.FgHiBlack).Sprintf("(%d shown of %d total)", shownCount, totalInGroup)))
	}).OrElse(func() {
		fmt.Fprint(out, fmt.Sprintf("%s %s\n", header, color.New(color.FgHiBlack).Sprintf("(%d items)", shownCount)))
	}).Run()
}

// outputGroupFooter outputs the footer for a group.
//
//nolint:gocritic // preferFprint
func outputGroupFooter(out io.Writer, result *storage.QueryResult, groupKey string, shownCount int) {
	totalInGroup := shownCount
	if groupMeta, ok := result.Meta["group_"+groupKey+"_total"].(int); ok {
		totalInGroup = groupMeta
	}

	if totalInGroup > shownCount {
		fmt.Fprintf(out, "  ... (%d more)\n", totalInGroup-shownCount)
	}
	fmt.Fprint(out, "\n")
}

// outputTableForObjects outputs a table for a list of objects
func outputTableForObjects(out io.Writer, cmd *cobra.Command, objectList []map[string]any, spec *objects.Spec, tableFieldOrder []string) {
	columnFields, columnWidths, columnHeaders := parseTableColumns(cmd, spec, tableFieldOrder)

	rowChan := make(chan []string)
	go func() {
		defer close(rowChan)
		for _, obj := range objectList {
			clipkg.AddDisplayFieldsForTable(obj, spec, cmd)
			values := make([]string, len(columnFields))
			for i, field := range columnFields {
				values[i] = clipkg.GetDisplayValue(obj, field, false)
			}
			rowChan <- values
		}
	}()

	_ = clipkg.StreamTable(out, "", columnHeaders, columnWidths, rowChan)
	fmt.Fprint(out, "\n")
}

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
	out := cmd.OutOrStdout()

	if len(result.Groups) == 0 {
		fmt.Fprint(out, "No objects found.\n")
		return nil
	}

	fmt.Fprint(out, "Objects grouped by kind:\n")
	if totalGroups, ok := result.Meta["total_groups"].(int); ok {
		fmt.Fprintf(out, "Total kinds: %d\n", totalGroups)
	}
	if totalCount, ok := result.Meta["total_count"].(int); ok {
		fmt.Fprintf(out, "Total objects: %d\n", totalCount)
	}
	fmt.Fprint(out, "\n")

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

		fmt.Fprintf(out, "Kind: %s (%d items)\n", kg.kind, len(kg.objects))

		// Load spec for this kind to get display_length defaults
		var spec *objects.Spec
		specLoader := objects.GetGlobalSpecLoader()
		if loadedSpec, err := specLoader.LoadSpecWithInheritance(kg.kind + ".yaml"); err == nil {
			spec = loadedSpec
		}

		if len(objectsToShow) > 0 {
			outputTableForObjects(out, cmd, objectsToShow, spec, tableFieldOrder)
		}
		if maxRows > 0 && len(kg.objects) > maxRows {
			fmt.Fprintf(out, "\n(showing first %d of %d; use --format json for full export)\n", maxRows, len(kg.objects))
		}
		fmt.Fprint(out, "\n")
	}

	return nil
}
