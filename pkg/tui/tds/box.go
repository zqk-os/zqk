package tds

import (
	"strings"

	"github.com/fatih/color"
)

// BorderStyle defines the glyphs used to draw panels and boxes.
type BorderStyle struct {
	TopLeft     string
	TopRight    string
	BottomLeft  string
	BottomRight string
	Horizontal  string
	Vertical    string
	TeeLeft     string
	TeeRight    string
}

var (
	// BorderRounded uses modern terminal rounded corners (recommended for dashboard cards).
	BorderRounded = BorderStyle{
		TopLeft:     "╭",
		TopRight:    "╮",
		BottomLeft:  "╰",
		BottomRight: "╯",
		Horizontal:  "─",
		Vertical:    "│",
		TeeLeft:     "├",
		TeeRight:    "┤",
	}

	// BorderLight uses standard single-line box drawing.
	BorderLight = BorderStyle{
		TopLeft:     "┌",
		TopRight:    "┐",
		BottomLeft:  "└",
		BottomRight: "┘",
		Horizontal:  "─",
		Vertical:    "│",
		TeeLeft:     "├",
		TeeRight:    "┤",
	}

	// BorderHeavy uses double-line borders (for primary header banners).
	BorderHeavy = BorderStyle{
		TopLeft:     "╔",
		TopRight:    "╗",
		BottomLeft:  "╚",
		BottomRight: "╝",
		Horizontal:  "═",
		Vertical:    "║",
		TeeLeft:     "╠",
		TeeRight:    "╣",
	}

	dimBorder = color.New(color.Faint).SprintFunc()
)

// Panel renders a titled box containing content lines.
// Every line is guaranteed to match width exactly.
func Panel(title string, lines []string, width int, style BorderStyle) string {
	if width < 20 {
		width = 80
	}

	var b strings.Builder
	innerWidth := width - 2

	// 1. Top border with optional embedded title
	b.WriteString(style.TopLeft)
	if title != "" {
		titleFormatted := " " + title + " "
		tw := VisibleWidth(titleFormatted)
		if tw < innerWidth-2 {
			b.WriteString(style.Horizontal)
			b.WriteString(titleFormatted)
			rem := innerWidth - tw - 1
			if rem > 0 {
				b.WriteString(strings.Repeat(style.Horizontal, rem))
			}
		} else {
			truncated := TruncateVisible(titleFormatted, innerWidth-2, "… ")
			b.WriteString(truncated)
			rem := innerWidth - VisibleWidth(truncated)
			if rem > 0 {
				b.WriteString(strings.Repeat(style.Horizontal, rem))
			}
		}
	} else {
		b.WriteString(strings.Repeat(style.Horizontal, innerWidth))
	}
	b.WriteString(style.TopRight + "\n")

	// 2. Content rows (flatten any embedded newlines to preserve left and right box borders)
	var flatLines []string
	for _, line := range lines {
		if strings.Contains(line, "\n") {
			parts := strings.Split(line, "\n")
			flatLines = append(flatLines, parts...)
		} else {
			flatLines = append(flatLines, line)
		}
	}

	for _, line := range flatLines {
		b.WriteString(style.Vertical)
		padded := PadRight(line, innerWidth)
		// If line exceeds inner width, safely truncate
		if VisibleWidth(padded) > innerWidth {
			padded = TruncateVisible(padded, innerWidth, "…")
			padded = PadRight(padded, innerWidth)
		}
		b.WriteString(padded)
		b.WriteString(style.Vertical + "\n")
	}

	// 3. Bottom border
	b.WriteString(style.BottomLeft)
	b.WriteString(strings.Repeat(style.Horizontal, innerWidth))
	b.WriteString(style.BottomRight + "\n")

	return b.String()
}

// SectionDivider renders a clean horizontal section divider line across width with a title.
func SectionDivider(title string, width int) string {
	if width < 20 {
		width = 80
	}
	if title == "" {
		return dimBorder(strings.Repeat("─", width)) + "\n"
	}

	prefix := "─── "
	tag := title + " "
	used := VisibleWidth(prefix) + VisibleWidth(tag)
	if used >= width {
		return dimBorder(TruncateVisible(prefix+tag, width, "…")) + "\n"
	}

	rem := width - used
	return dimBorder(prefix) + tag + dimBorder(strings.Repeat("─", rem)) + "\n"
}
