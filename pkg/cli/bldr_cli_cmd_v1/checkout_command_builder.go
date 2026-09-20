package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewCheckoutCommandBuilder creates a new checkout command
func NewCheckoutCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("checkout")
	builder.WithShort("Checkout committed SHA into .zqk/local-ci/workdir")
	help := clipkg.DynamicHelpBuilder("Checkout committed SHA into .zqk/local-ci/workdir")
	help.WithDescriptionLines("Refresh the Local CI git worktree only (no scan-tests).")
	help.AddExample("Checkout HEAD", "%s ci checkout")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.AddStringFlag("sha", "", "", "Commit SHA to checkout (default: HEAD)")
	builder.AddBoolFlag("allow-dirty", "", false, "Allow checkout when studio has tracked dirty files")
	builder.AddBoolFlag("no-archive", "", false, "Skip tar drop / archive-git recording")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
