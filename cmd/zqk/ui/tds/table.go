package tds

import (
	"strings"

	"github.com/fatih/color"
)

// Alignment specifies cell alignment within a table column.
type Alignment int

const (
	AlignLeft Alignment = iota
	AlignCenter
	AlignRight
)

// Column defines a column specification in a TDS Table.
type Column struct {
	Title     string
	Align     Alignment
	MinWidth  int
	Weight    float64 // Relative flex weight when distributing available width
	allocated int     // Calculated visible width during Render
}

// Table provides flex-width, ANSI-safe tabular rendering.
type Table struct {
	TotalWidth int
	Columns    []Column
	Rows       [][]string
	dimLine    func(a ...interface{}) string
	boldTitle  func(a ...interface{}) string
}

// NewTable initializes a new table targeted to totalWidth.
func NewTable(totalWidth int) *Table {
	if totalWidth < 40 {
		totalWidth = 80
	}
	return &Table{
		TotalWidth: totalWidth,
		Columns:    make([]Column, 0),
		Rows:       make([][]string, 0),
		dimLine:    color.New(color.Faint).SprintFunc(),
		boldTitle:  color.New(color.FgWhite, color.Bold).SprintFunc(),
	}
}

// AddColumn appends a column definition.
func (t *Table) AddColumn(title string, align Alignment, minWidth int, weight float64) *Table {
	if minWidth < 3 {
		minWidth = 3
	}
	if weight <= 0 {
		weight = 1.0
	}
	t.Columns = append(t.Columns, Column{
		Title:    title,
		Align:    align,
		MinWidth: minWidth,
		Weight:   weight,
	})
	return t
}

// AddRow appends a row of cell strings.
func (t *Table) AddRow(cells ...string) *Table {
	t.Rows = append(t.Rows, cells)
	return t
}

// Render compiles the table into an ANSI-formatted string.
func (t *Table) Render() string {
	numCols := len(t.Columns)
	if numCols == 0 {
		return ""
	}

	// 1. Calculate column widths
	// Overhead: "│ " at left, " │ " between cols, " │" at right -> 1 + (numCols)*3
	overhead := 1 + numCols*3
	availWidth := t.TotalWidth - overhead
	if availWidth < numCols*3 {
		availWidth = numCols * 3
	}

	totalWeight := 0.0
	for _, col := range t.Columns {
		totalWeight += col.Weight
	}

	remaining := availWidth
	for i := range t.Columns {
		calc := int(float64(availWidth) * (t.Columns[i].Weight / totalWeight))
		if calc < t.Columns[i].MinWidth {
			calc = t.Columns[i].MinWidth
		}
		t.Columns[i].allocated = calc
		remaining -= calc
	}

	// Distribute any rounding remainder to the first flexible column
	if remaining > 0 && len(t.Columns) > 0 {
		t.Columns[0].allocated += remaining
	}

	var b strings.Builder

	// Header Divider
	t.renderHorizontalDivider(&b, "┌", "┬", "┐")

	// Header Row
	b.WriteString(t.dimLine("│ "))
	for i, col := range t.Columns {
		paddedTitle := t.formatCell(t.boldTitle(col.Title), col.allocated, col.Align)
		b.WriteString(paddedTitle)
		if i < numCols-1 {
			b.WriteString(t.dimLine(" │ "))
		}
	}
	b.WriteString(t.dimLine(" │\n"))

	// Header-to-Data Divider
	t.renderHorizontalDivider(&b, "├", "┼", "┤")

	// Data Rows
	for _, row := range t.Rows {
		b.WriteString(t.dimLine("│ "))
		for i, col := range t.Columns {
			cellContent := ""
			if i < len(row) {
				cellContent = row[i]
			}
			paddedCell := t.formatCell(cellContent, col.allocated, col.Align)
			b.WriteString(paddedCell)
			if i < numCols-1 {
				b.WriteString(t.dimLine(" │ "))
			}
		}
		b.WriteString(t.dimLine(" │\n"))
	}

	// Bottom Divider
	t.renderHorizontalDivider(&b, "└", "┴", "┘")

	return b.String()
}

func (t *Table) formatCell(content string, width int, align Alignment) string {
	vw := VisibleWidth(content)
	if vw > width {
		content = TruncateVisible(content, width, "…")
		vw = VisibleWidth(content)
	}

	diff := width - vw
	if diff <= 0 {
		return content
	}

	switch align {
	case AlignRight:
		return strings.Repeat(" ", diff) + content
	case AlignCenter:
		left := diff / 2
		right := diff - left
		return strings.Repeat(" ", left) + content + strings.Repeat(" ", right)
	case AlignLeft:
		fallthrough
	default:
		return content + strings.Repeat(" ", diff)
	}
}

func (t *Table) renderHorizontalDivider(b *strings.Builder, left, mid, right string) {
	b.WriteString(t.dimLine(left))
	for i, col := range t.Columns {
		b.WriteString(t.dimLine(strings.Repeat("─", col.allocated+2)))
		if i < len(t.Columns)-1 {
			b.WriteString(t.dimLine(mid))
		}
	}
	b.WriteString(t.dimLine(right + "\n"))
}
