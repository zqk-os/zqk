package precommit

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/precommit"
)

// NewAggregateCmd creates the pre-commit aggregate command from spec-driven builder; RunE reads flags and runs aggregate.
func NewAggregateCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewPreCommitAggregateCommandBuilder()
	cmd.RunE = runPreCommitAggregate
	return cmd
}

func runPreCommitAggregate(cmd *cobra.Command, _ []string) error {
	projectRoot, err := resolvePreCommitProjectRoot(cmd)
	if err != nil {
		return err
	}
	_, err = precommit.Aggregate(projectRoot)
	return err
}
