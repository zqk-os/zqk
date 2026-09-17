package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/jedib0t/go-pretty/v6/text"
)

// StreamTable formats a table line-by-line to an io.Writer using go-pretty StyleLight.
// It uses exact column widths and truncates strings to avoid building the entire table in memory.
func StreamTable(out io.Writer, title string, columns []string, widths []int, rowChan <-chan []string) error {
	if title != "" {
		fmt.Fprintln(out, title)
	}

	printTop := func() {
		fmt.Fprint(out, "┌")
		for i, w := range widths {
			fmt.Fprint(out, strings.Repeat("─", w+2))
			if i < len(widths)-1 {
				fmt.Fprint(out, "┬")
			}
		}
		fmt.Fprintln(out, "┐")
	}

	printMiddle := func() {
		fmt.Fprint(out, "├")
		for i, w := range widths {
			fmt.Fprint(out, strings.Repeat("─", w+2))
			if i < len(widths)-1 {
				fmt.Fprint(out, "┼")
			}
		}
		fmt.Fprintln(out, "┤")
	}

	printBottom := func() {
		fmt.Fprint(out, "└")
		for i, w := range widths {
			fmt.Fprint(out, strings.Repeat("─", w+2))
			if i < len(widths)-1 {
				fmt.Fprint(out, "┴")
			}
		}
		fmt.Fprintln(out, "┘")
	}

	printRow := func(cells []string) {
		fmt.Fprint(out, "│")
		for i, w := range widths {
			cell := ""
			if i < len(cells) {
				cell = cells[i]
			}
			cell = TruncateString(cell, w)
			// text.Pad right pads the string to width
			padded := text.Pad(cell, w, ' ')
			fmt.Fprint(out, " "+padded+" │")
		}
		fmt.Fprintln(out)
	}

	printTop()

	headers := make([]string, len(columns))
	for i, c := range columns {
		headers[i] = strings.ToUpper(c)
	}
	printRow(headers)
	printMiddle()

	for row := range rowChan {
		printRow(row)
	}

	printBottom()
	return nil
}
