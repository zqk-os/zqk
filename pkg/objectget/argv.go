package objectget

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/paths"
)

// Options configures argv construction for `object get`.
type Options struct {
	// Binary is the CLI executable name (default: paths.CLICommandName).
	Binary string
	// Format is optional (--format json|yaml|table|...).
	Format string
	// View is optional (--view); empty means default resolver/view semantics.
	View string
	// Hydration optionally adds --link-hydration when non-Unspecified.
	Hydration LinkHydration
}

// BuildObjectGetArgv returns argv for ` [<binary>] object get <id> [flags]`.
// The first element is always Binary (default paths.CLICommandName).
func BuildObjectGetArgv(id string, opts Options) []string {
	bin := strings.TrimSpace(opts.Binary)
	if bin == "" {
		bin = paths.CLICommandName
	}
	if strings.TrimSpace(id) == "" {
		return []string{bin}
	}

	args := []string{bin, "object", "get", strings.TrimSpace(id)}
	if f := strings.TrimSpace(opts.Format); f != "" {
		args = append(args, "--format", f)
	}
	if v := strings.TrimSpace(opts.View); v != "" && v != ViewDefault {
		args = append(args, "--view", v)
	}
	if opts.Hydration != HydrationUnspecified {
		if s := opts.Hydration.String(); s != "" {
			args = append(args, "--link-hydration", s)
		}
	}
	return args
}
