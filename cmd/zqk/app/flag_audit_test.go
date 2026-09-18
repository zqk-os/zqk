package app

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// TestFlagAudit ensures that all commands across all subsystems register their flags properly
// and follow common flag conventions.
func TestFlagAudit(t *testing.T) {
	// Initialize root command and register all dynamic and static subcommands
	root := NewRootCommand()

	// Recursively collect all commands
	var allCmds []*cobra.Command
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		allCmds = append(allCmds, c)
		for _, child := range c.Commands() {
			walk(child)
		}
	}
	walk(root)

	// Subsystems or common flag names we might want to check
	// e.g. every command shouldn't have duplicate shorthand flags, etc.
	for _, cmd := range allCmds {
		// Skip root if it has no run, or help/version commands
		if cmd.Name() == "help" || cmd.Name() == "version" {
			continue
		}

		// Basic check: commands shouldn't panic when we check their flags
		flags := cmd.Flags()
		if flags == nil {
			t.Errorf("Command %q has nil FlagSet", cmd.CommandPath())
			continue
		}

		// Ensure that flags can be iterated over and have valid configurations
		flags.VisitAll(func(f *pflag.Flag) {
			if f.Usage == "" {
				t.Errorf("Command %q flag %q is missing usage description", cmd.CommandPath(), f.Name)
			}
			// Check for problematic flag names
			if strings.Contains(f.Name, "_") {
				t.Errorf("Command %q flag %q uses underscore, prefer hyphens", cmd.CommandPath(), f.Name)
			}
		})
	}
}
