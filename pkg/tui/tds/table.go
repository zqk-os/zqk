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
	if weight <= 0.0001 {
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
	// Overhead: "│ " at left (2), " │ " between cols (3 each), " │\n" at right (2) -> 1 + numCols*3
	overhead := 1 + numCols*3
	availWidth := t.TotalWidth - overhead
	if availWidth < numCols {
		availWidth = numCols
	}

	totalWeight := 0.0
	totalMin := 0
	for _, col := range t.Columns {
		totalWeight += col.Weight
		totalMin += col.MinWidth
	}
	if totalWeight <= 0 {
		totalWeight = float64(numCols)
	}

	if availWidth >= totalMin {
		// Normal case: Room to satisfy all MinWidths.
		// Allocate MinWidth first, then distribute extra space proportionally to Weight.
		extra := availWidth - totalMin
		remainingExtra := extra
		for i := range t.Columns {
			add := int(float64(extra) * (t.Columns[i].Weight / totalWeight))
			t.Columns[i].allocated = t.Columns[i].MinWidth + add
			remainingExtra -= add
		}
		// Distribute any rounding remainder
		for i := 0; remainingExtra > 0 && i < len(t.Columns); i++ {
			t.Columns[i].allocated++
			remainingExtra--
		}
	} else {
		// Constrained case: Available width is less than totalMin.
		// Scale each column proportionally down according to Weight, with a minimum floor of 1.
		floor := 1
		remaining := availWidth
		for i := range t.Columns {
			calc := int(float64(availWidth) * (t.Columns[i].Weight / totalWeight))
			if calc < floor {
				calc = floor
			}
			t.Columns[i].allocated = calc
			remaining -= calc
		}
		if remaining > 0 {
			for i := 0; remaining > 0 && i < len(t.Columns); i++ {
				t.Columns[i].allocated++
				remaining--
			}
		} else if remaining < 0 {
			for i := len(t.Columns) - 1; remaining < 0 && i >= 0; i-- {
				if t.Columns[i].allocated > floor {
					reduce := t.Columns[i].allocated - floor
					if -remaining < reduce {
						reduce = -remaining
					}
					t.Columns[i].allocated -= reduce
					remaining += reduce
				}
			}
		}
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
	content = strings.ReplaceAll(content, "\r\n", " ")
	content = strings.ReplaceAll(content, "\n", " ")
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
