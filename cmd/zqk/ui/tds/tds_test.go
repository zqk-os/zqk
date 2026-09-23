package tds

import (
	"strings"
	"testing"

	"github.com/fatih/color"
	"github.com/stretchr/testify/assert"
)

func TestVisibleWidth(t *testing.T) {
	// Plain ASCII
	assert.Equal(t, 5, VisibleWidth("hello"))

	// ANSI colored string (explicit escape sequence)
	colored := "\x1b[32mhello\x1b[0m"
	assert.Equal(t, 5, VisibleWidth(colored))
	assert.Greater(t, len(colored), 5) // byte length is 14, visible width is 5

	// Emojis / wide runes
	assert.Equal(t, 2, VisibleWidth("⚡"))
	assert.Equal(t, 2, VisibleWidth("📊"))
}

func TestPadding(t *testing.T) {
	colored := color.GreenString("test") // 4 visual cells
	paddedR := PadRight(colored, 10)
	assert.Equal(t, 10, VisibleWidth(paddedR))

	paddedL := PadLeft(colored, 10)
	assert.Equal(t, 10, VisibleWidth(paddedL))

	paddedC := PadCenter(colored, 10)
	assert.Equal(t, 10, VisibleWidth(paddedC))
}

func TestTruncateVisible(t *testing.T) {
	colored := color.RedString("very long error message that exceeds width")
	truncated := TruncateVisible(colored, 15, "…")
	assert.LessOrEqual(t, VisibleWidth(truncated), 15)
	assert.True(t, strings.HasSuffix(StripANSI(truncated), "…"))
	// Ensure reset escape sequence exists at the end
	assert.True(t, strings.Contains(truncated, "\x1b[0m"))
}

func TestPanel_LineUniformity(t *testing.T) {
	targetWidth := 80
	lines := []string{
		color.CyanString("Line 1 with colored content"),
		"Line 2: regular plain text",
		"Line 3: ⚡ emoji and " + color.YellowString("bold numbers: 12345"),
		"Line 4: " + Badge("PASS") + " - " + Badge("WARN"),
	}

	rendered := Panel("TEST PANEL", lines, targetWidth, BorderRounded)
	splitLines := strings.Split(strings.TrimRight(rendered, "\n"), "\n")

	// Verify that EVERY line in the panel matches targetWidth exactly
	for i, l := range splitLines {
		vw := VisibleWidth(l)
		assert.Equal(t, targetWidth, vw, "Panel line %d visible width mismatch. Line: %q", i, l)
	}
}

func TestTable_LineUniformityAndGridAlignment(t *testing.T) {
	targetWidth := 90
	tbl := NewTable(targetWidth).
		AddColumn("JOB IDENTIFIER", AlignLeft, 20, 0.40).
		AddColumn("RUNS", AlignRight, 8, 0.15).
		AddColumn("STATUS", AlignCenter, 10, 0.20).
		AddColumn("TREND", AlignCenter, 12, 0.25)

	tbl.AddRow("SCH-cache-prewarm", "1,481", Badge("PASS"), Sparkline([]float64{1, 2, 5, 2, 8}, 5))
	tbl.AddRow(color.CyanString("SCH-autofix-run"), "953", Badge("WARN"), Sparkline([]float64{4, 4, 6, 8, 3}, 5))
	tbl.AddRow("SCH-extremely-long-job-name-that-will-be-truncated-properly", "12", Badge("FAIL"), Sparkline([]float64{9, 9, 9}, 5))

	rendered := tbl.Render()
	splitLines := strings.Split(strings.TrimRight(rendered, "\n"), "\n")

	// Verify every line has the exact same visual width
	expectedWidth := VisibleWidth(splitLines[0])
	assert.GreaterOrEqual(t, expectedWidth, targetWidth-2)

	for i, l := range splitLines {
		vw := VisibleWidth(l)
		assert.Equal(t, expectedWidth, vw, "Table line %d visible width mismatch. Line: %q", i, l)
	}
}

func TestComponents(t *testing.T) {
	bPass := Badge("PASS")
	assert.Equal(t, 8, VisibleWidth(bPass)) // "[✓ PASS]" is 8 visible cells

	bar := ProgressBar(0.65, 10)
	assert.Contains(t, bar, "65%")

	spark := Sparkline([]float64{10, 20, 30, 40}, 4)
	assert.Equal(t, 6, VisibleWidth(spark)) // "[ ▄▆█]" is 6 cells
}
