package cli

import clipkg "github.com/zqk-os/zqk/pkg/cli"

// StandardColorPrinters returns commonly used color sprint functions (cyan, green, yellow).
func StandardColorPrinters() (cyan, green, yellow func(a ...any) string) {
	return clipkg.StandardColorPrinters()
}
