package precommit

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/precommit"
)

const emptyValue = ""

// NewAggregateCmd creates the pre-commit aggregate command from spec-driven builder; RunE reads flags and runs aggregate.
func NewAggregateCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewPreCommitAggregateCommandBuilder()
	cmd.RunE = runPreCommitAggregate
	return cmd
}

func runPreCommitAggregate(cmd *cobra.Command, _ []string) error {
	projectRoot, _ := cmd.Flags().GetString("project-root")
	if projectRoot == emptyValue {
		projectRoot = cli.ResolveProjectRoot(".")
	}
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found; run from repo or pass --project-root")
	}
	_, err := precommit.Aggregate(projectRoot)
	return err
}
