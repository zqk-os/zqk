package tds

import (
	"regexp"
	"strings"

	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

// GetTerminalWidth retrieves the current terminal column width, falling back to fallback if unavailable or smaller than 40.
func GetTerminalWidth(fallback int) int {
	if fallback < 40 {
		fallback = 80
	}
	if w, _, err := term.GetSize(0); err == nil && w >= 60 {
		return w
	}
	return fallback
}

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

// IsWideIcon returns true if the symbol contains emoji or pictographic runes
// that terminal emulators commonly render with a wide bounding box or 2 character cells.
func IsWideIcon(symbol string) bool {
	for _, r := range symbol {
		// Variation selector 16 (emoji presentation)
		if r == 0xfe0f {
			return true
		}
		// Emoji & Symbols ranges:
		// Miscellaneous Technical (0x2300-0x23FF) e.g. ⏱ (0x23F1)
		// Miscellaneous Symbols (0x2600-0x26FF) e.g. ⚙ (0x2699), ⚠️ (0x26A0)
		// Supplemental Symbols and Pictographs / Emoji (0x1F000-0x1FAFF) e.g. 🛡 (0x1F6E1), 📦, 🤖, etc.
		if (r >= 0x2300 && r <= 0x23ff) ||
			(r >= 0x2600 && r <= 0x27bf && r != 0x2713 && r != 0x2717 && r != 0x2794) ||
			(r >= 0x1f000 && r <= 0x1faff) {
			return true
		}
	}
	return false
}

// IconPad inspects the symbol and returns it with terminal-safe trailing whitespace.
// Wide icons receive 2 trailing spaces to prevent text smooshing in modern terminals,
// while compact symbols receive 1 space.
func IconPad(symbol string) string {
	if symbol == "" {
		return ""
	}
	if IsWideIcon(symbol) {
		return symbol + "  "
	}
	return symbol + " "
}

// Icon renders a symbol and label with consistent terminal-safe spacing.
func Icon(symbol, label string) string {
	if symbol == "" {
		return label
	}
	return IconPad(symbol) + label
}
