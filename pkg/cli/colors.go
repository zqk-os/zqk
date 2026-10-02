package cli

import "github.com/fatih/color"

// StandardColorPrinters returns commonly used color sprint functions (cyan, green, yellow).
func StandardColorPrinters() (cyan, green, yellow func(a ...any) string) {
	return color.New(color.FgCyan).SprintFunc(),
		color.New(color.FgGreen).SprintFunc(),
		color.New(color.FgYellow).SprintFunc()
}

// UIPalette holds standard terminal color printing functions.
type UIPalette struct {
	Cyan   func(a ...any) string
	Green  func(a ...any) string
	Yellow func(a ...any) string
	Bold   func(a ...any) string
}

// StandardUIPalette returns commonly used terminal text formatters.
func StandardUIPalette() UIPalette {
	return UIPalette{
		Cyan:   color.New(color.FgCyan).SprintFunc(),
		Green:  color.New(color.FgGreen).SprintFunc(),
		Yellow: color.New(color.FgYellow).SprintFunc(),
		Bold:   color.New(color.Bold).SprintFunc(),
	}
}
