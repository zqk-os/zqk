package cli

import "github.com/fatih/color"

// StandardColorPrinters returns commonly used color sprint functions (cyan, green, yellow).
func StandardColorPrinters() (cyan, green, yellow func(a ...any) string) {
	return color.New(color.FgCyan).SprintFunc(),
		color.New(color.FgGreen).SprintFunc(),
		color.New(color.FgYellow).SprintFunc()
}
