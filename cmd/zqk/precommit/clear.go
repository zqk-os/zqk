package precommit

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/precommit"
)

// NewClearCmd creates the pre-commit clear command from spec-driven builder; RunE reads flags and clears results.
func NewClearCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewPreCommitClearCommandBuilder()
	cmd.RunE = runPreCommitClear
	return cmd
}

func runPreCommitClear(cmd *cobra.Command, _ []string) error {
	projectRoot, _ := cmd.Flags().GetString("project-root")
	if projectRoot == emptyValue {
		projectRoot = cli.ResolveProjectRoot(".")
	}
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found; run from repo or pass --project-root")
	}
	return precommit.Clear(projectRoot)
}
