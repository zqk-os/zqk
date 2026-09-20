package app

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// TestFlagCollisions walks the entire CLI command tree and verifies
// that no command redefines a shorthand flag that is already used
// by a persistent (inherited) flag. This prevents silent panics
// during cobra.GenBashCompletion.
func TestFlagCollisions(t *testing.T) {
	rootCmd := NewRootCommand()

	var checkCmd func(cmd *cobra.Command)
	checkCmd = func(cmd *cobra.Command) {
		// Cobra panics if local flags and persistent flags use the same shorthand
		// when GenBashCompletion tries to merge them. We simulate that merge check here.
		inherited := cmd.InheritedFlags()
		local := cmd.LocalFlags()

		if inherited != nil && local != nil {
			inherited.VisitAll(func(ih *pflag.Flag) {
				if ih.Shorthand != "" && ih.Shorthand != "h" { // 'h' is often handled gracefully or excluded
					localFlag := local.ShorthandLookup(ih.Shorthand)
					if localFlag != nil {
						t.Errorf("Flag shorthand collision detected in command %q: shorthand '-%s' is used by local flag '--%s' and inherited flag '--%s'",
							cmd.CommandPath(), ih.Shorthand, localFlag.Name, ih.Name)
					}
				}
			})
		}

		for _, child := range cmd.Commands() {
			checkCmd(child)
		}
	}

	checkCmd(rootCmd)
}
