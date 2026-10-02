package precommit

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/precommit"
)

// NewClearCmd creates the pre-commit clear command from spec-driven builder; RunE reads flags and clears results.
func NewClearCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewPreCommitClearCommandBuilder()
	cmd.RunE = runPreCommitClear
	return cmd
}

func runPreCommitClear(cmd *cobra.Command, _ []string) error {
	projectRoot, err := resolvePreCommitProjectRoot(cmd)
	if err != nil {
		return err
	}
	return precommit.Clear(projectRoot)
}
