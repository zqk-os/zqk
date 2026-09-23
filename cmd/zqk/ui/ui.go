package ui

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

// NewUICmd creates the 'zqk ui' command for interactive terminal mission control.
func NewUICmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewUiCommandBuilder()
	cmd.Aliases = []string{"dashboard", "console"}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
			projectRoot := proc.ProjectRoot()
			if projectRoot == "" {
				projectRoot = cli.ResolveProjectRoot(".")
			}

			tab, _ := cmd.Flags().GetString("tab")
			sec := pkgctx.NewSystemSecurityContext()
			sp := proc.Storage()

			return RunTUI(proc.OperationContext(), projectRoot, tab, sp, sec)
		})(cmd, args)
	}

	return cmd
}
