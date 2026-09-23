package tds

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
)

var (
	badgePassStyle = color.New(color.FgGreen, color.Bold).SprintFunc()
	badgeWarnStyle = color.New(color.FgYellow, color.Bold).SprintFunc()
	badgeFailStyle = color.New(color.FgRed, color.Bold).SprintFunc()
	badgeInfoStyle = color.New(color.FgCyan).SprintFunc()
	badgeDimStyle  = color.New(color.Faint).SprintFunc()

	statLabelStyle = color.New(color.Faint).SprintFunc()
	statValueStyle = color.New(color.FgWhite, color.Bold).SprintFunc()
)

// Badge renders a standardized status pill.
func Badge(status string) string {
	switch strings.ToUpper(status) {
	case "PASS", "HEALTHY", "ACTIVE", "OK", "DONE", "ENFORCING":
		return badgePassStyle("[✓ " + status + "]")
	case "WARN", "ATTENTION", "DEGRADED", "STALE":
		return badgeWarnStyle("[⚠️  " + status + "]")
	case "FAIL", "BLOCKED", "ERROR", "CRITICAL":
		return badgeFailStyle("[✗ " + status + "]")
	case "IDLE", "DRAFT", "PENDING", "UNKNOWN":
		return badgeDimStyle("[" + status + "]")
	default:
		return badgeInfoStyle("[" + status + "]")
	}
}

// RowCursor renders the selection carrot/pointer indicator with consistent alignment.
// Selected rows display a bright cyan "> " prefix, while unselected rows display "  " padding
// so all row text aligns cleanly across columns.
func RowCursor(isSelected bool, id string) string {
	if isSelected {
		return color.New(color.FgCyan, color.Bold).Sprint("> ") + id
	}
	return "  " + id
}

// StatItem represents a single key-value metric pair.
type StatItem struct {
	Label string
	Value string
	Extra string
}

// StatRow renders a 2-column or 4-column balanced key-value metric line.
func StatRow(items []StatItem, totalWidth int) string {
	if len(items) == 0 {
		return ""
	}
	colWidth := totalWidth / len(items)
	if colWidth < 20 {
		colWidth = 20
	}

	var b strings.Builder
	for i, item := range items {
		content := statLabelStyle(item.Label+": ") + statValueStyle(item.Value)
		if item.Extra != "" {
			content += " " + item.Extra
		}
		padded := PadRight(content, colWidth)
		if i == len(items)-1 {
			b.WriteString(content) // don't trail-pad the last item
		} else {
			b.WriteString(padded)
		}
	}
	return b.String()
}

// ProgressBar renders a normalized graphical bar: [██████░░░░] 60%
func ProgressBar(fraction float64, barWidth int) string {
	if fraction < 0 {
		fraction = 0
	}
	if fraction > 1.0 {
		fraction = 1.0
	}
	if barWidth < 5 {
		barWidth = 10
	}

	filledLen := int(float64(barWidth) * fraction)
	emptyLen := barWidth - filledLen

	filled := strings.Repeat("█", filledLen)
	empty := strings.Repeat("░", emptyLen)
	pct := fmt.Sprintf(" %3.0f%%", fraction*100)

	barColor := color.FgGreen
	if fraction > 0.85 {
		barColor = color.FgYellow
	}
	if fraction > 0.95 {
		barColor = color.FgRed
	}

	return color.New(barColor).Sprint("["+filled) + color.New(color.Faint).Sprint(empty+"]") + pct
}

// Sparkline renders an 8-level trendline from numeric data points.
func Sparkline(values []float64, maxLen int) string {
	if len(values) == 0 {
		return color.New(color.Faint).Sprint("[--]")
	}

	if maxLen > 0 && len(values) > maxLen {
		values = values[len(values)-maxLen:]
	}

	minVal, maxVal := values[0], values[0]
	for _, v := range values {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}

	levels := []rune{' ', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
	rangeVal := maxVal - minVal

	var b strings.Builder
	b.WriteRune('[')
	for _, v := range values {
		if rangeVal == 0 {
			b.WriteRune('▄')
			continue
		}
		idx := int(((v - minVal) / rangeVal) * float64(len(levels)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(levels) {
			idx = len(levels) - 1
		}
		b.WriteRune(levels[idx])
	}
	b.WriteRune(']')

	return color.New(color.FgCyan).Sprint(b.String())
}
