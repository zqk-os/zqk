package tds

import (
	"regexp"
	"strings"

	"github.com/mattn/go-runewidth"
)

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// StripANSI removes all ANSI escape sequences from a string.
func StripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

// VisibleWidth returns the true terminal column width of a string,
// properly discounting ANSI escape sequences and accounting for double-width runes/emojis.
func VisibleWidth(s string) int {
	clean := StripANSI(s)
	return runewidth.StringWidth(clean)
}

// PadRight pads a string with spaces until its visible width matches targetWidth.
// If the string already exceeds targetWidth, it is returned as-is.
func PadRight(s string, targetWidth int) string {
	vw := VisibleWidth(s)
	if vw >= targetWidth {
		return s
	}
	return s + strings.Repeat(" ", targetWidth-vw)
}

// PadLeft pads a string with spaces on the left until its visible width matches targetWidth.
func PadLeft(s string, targetWidth int) string {
	vw := VisibleWidth(s)
	if vw >= targetWidth {
		return s
	}
	return strings.Repeat(" ", targetWidth-vw) + s
}

// PadCenter centers a string within targetWidth.
func PadCenter(s string, targetWidth int) string {
	vw := VisibleWidth(s)
	if vw >= targetWidth {
		return s
	}
	diff := targetWidth - vw
	left := diff / 2
	right := diff - left
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}

// TruncateVisible safely truncates a string so that its visible width does not exceed maxWidth.
// If truncation occurs, the tail suffix (e.g. "…") is appended, and an ANSI reset code
// is guaranteed to prevent color styling from bleeding into adjacent columns.
func TruncateVisible(s string, maxWidth int, tail string) string {
	if maxWidth <= 0 {
		return ""
	}
	vw := VisibleWidth(s)
	if vw <= maxWidth {
		return s
	}

	tailWidth := VisibleWidth(tail)
	budget := maxWidth - tailWidth
	if budget <= 0 {
		return PadRight(tail, maxWidth)
	}

	// Walk characters while tracking visible width and ANSI sequences.
	var b strings.Builder
	currentWidth := 0
	inEsc := false

	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == 0x1b {
			inEsc = true
			b.WriteRune(r)
			continue
		}
		if inEsc {
			b.WriteRune(r)
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}

		rw := runewidth.RuneWidth(r)
		if currentWidth+rw > budget {
			break
		}
		b.WriteRune(r)
		currentWidth += rw
	}

	b.WriteString("\x1b[0m") // Reset styling
	b.WriteString(tail)
	return b.String()
}
