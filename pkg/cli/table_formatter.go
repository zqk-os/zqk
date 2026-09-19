package cli

import (
	"strconv"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/spf13/cobra"
)

const columnsFlagName = "columns"

// ParseColumnsFlag parses the --columns flag value (format: "id:3,status:5,title:50")
// Returns a map of field name to column width
func ParseColumnsFlag(columnsStr string) (map[string]int, error) {
	if columnsStr == emptyValue {
		return nil, nil
	}

	columns := make(map[string]int)
	for part := range strings.SplitSeq(columnsStr, ",") {
		part = strings.TrimSpace(part)
		if part == emptyValue {
			continue
		}

		// Split on colon
		fieldPart, widthPart, ok := strings.Cut(part, ":")
		if !ok {
			return nil, errfmt.Errorf("invalid column format: %s (expected field:width)", part)
		}

		fieldName := strings.TrimSpace(fieldPart)
		widthStr := strings.TrimSpace(widthPart)

		width, err := strconv.Atoi(widthStr)
		if err != nil {
			return nil, errfmt.Errorf("invalid column width for %s: %s (must be integer)", fieldName, widthStr)
		}

		if width <= 0 {
			return nil, errfmt.Errorf("column width for %s must be positive, got %d", fieldName, width)
		}

		columns[fieldName] = width
	}

	return columns, nil
}

// GetColumnWidth gets the column width for a field, checking in order:
// 1. --columns flag override (highest priority)
// 2. display_length from spec
// 3. default value
func GetColumnWidth(fieldName string, cmd *cobra.Command, spec *objects.Spec, defaultWidth int) int {
	// Check --columns flag first (highest priority)
	columnsStr, err := cmd.Flags().GetString(columnsFlagName)
	if err == nil && columnsStr != emptyValue {
		columns, err := ParseColumnsFlag(columnsStr)
		if err == nil {
			if width, ok := columns[fieldName]; ok {
				return width
			}
		}
	}

	// Check --fields StringArray flag for embedded widths (e.g., --fields id:55,title:100)
	if fieldsArr, err := cmd.Flags().GetStringArray("fields"); err == nil && len(fieldsArr) > 0 {
		for _, s := range fieldsArr {
			for _, part := range strings.Split(s, ",") {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
				fieldPart, widthPart, ok := strings.Cut(part, ":")
				if ok && strings.TrimSpace(fieldPart) == fieldName {
					if width, parseErr := strconv.Atoi(strings.TrimSpace(widthPart)); parseErr == nil && width > 0 {
						return width
					}
				}
			}
		}
	}

	// Check display_length from spec
	if spec != nil {
		if width := objects.GetDisplayLength(spec, fieldName, 0); width > 0 {
			return width
		}
	}

	// Fall back to default
	return defaultWidth
}

// TruncateString truncates a string to the specified length, adding "..." if truncated
func TruncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// RenderTable formats a table using go-pretty/table with specified column widths.
func RenderTable(columns []string, widths []int, rows [][]string) string {
	return RenderTableWithTitle("", columns, widths, rows)
}

// RenderTableWithTitle formats a table using go-pretty/table with specified column widths and a centered title row.
func RenderTableWithTitle(title string, columns []string, widths []int, rows [][]string) string {
	t := table.NewWriter()
	if title != "" {
		t.SetTitle(title)
	}

	// Prepare header
	headerRow := make(table.Row, len(columns))
	for i, h := range columns {
		headerRow[i] = strings.ToUpper(h)
	}
	t.AppendHeader(headerRow)

	// Prepare rows
	for _, r := range rows {
		row := make(table.Row, len(r))
		for i, v := range r {
			row[i] = v
		}
		t.AppendRow(row)
	}

	// Prepare column configs
	var colConfigs []table.ColumnConfig
	for i, w := range widths {
		colConfigs = append(colConfigs, table.ColumnConfig{
			Number:           i + 1,
			WidthMin:         w,
			WidthMax:         w,
			WidthMaxEnforcer: TruncateString,
		})
	}
	t.SetColumnConfigs(colConfigs)

	t.SetStyle(table.StyleLight)
	return t.Render()
}

// RenderTableWithTitleAndWrap formats a table using go-pretty/table with specified column widths, centered title, and text wrapping (instead of truncation).
func RenderTableWithTitleAndWrap(title string, columns []string, widths []int, rows [][]string) string {
	t := table.NewWriter()
	if title != "" {
		t.SetTitle(title)
	}

	// Prepare header
	headerRow := make(table.Row, len(columns))
	for i, h := range columns {
		headerRow[i] = strings.ToUpper(h)
	}
	t.AppendHeader(headerRow)

	// Prepare rows
	for _, r := range rows {
		row := make(table.Row, len(r))
		for i, v := range r {
			row[i] = v
		}
		t.AppendRow(row)
	}

	// Prepare column configs - set WidthMin and WidthMax to enforce exact widths and wrapping
	var colConfigs []table.ColumnConfig
	for i, w := range widths {
		colConfigs = append(colConfigs, table.ColumnConfig{
			Number:           i + 1,
			WidthMin:         w,
			WidthMax:         w,
			WidthMaxEnforcer: text.WrapSoft,
		})
	}
	t.SetColumnConfigs(colConfigs)

	t.SetStyle(table.StyleLight)
	t.Style().Title.Colors = text.Colors{text.Bold}
	t.Style().Color.Header = text.Colors{text.Bold}
	return t.Render()
}
