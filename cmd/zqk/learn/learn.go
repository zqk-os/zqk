package learn

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/paths"
)

// NewLearnCmd creates the learn command for interactive curriculum mode
func NewLearnCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewLearnCommandBuilder(), &cobra.Command{
		Use:   "learn",
		Short: "Interactive curriculum mode",
		Long:  paths.RewriteCanonicalCLIInvocations(`Provides an interactive curriculum mode to learn the zqk system and concepts.`),
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Println("Interactive curriculum mode coming soon.")
		},
	})

	return cmd
}
