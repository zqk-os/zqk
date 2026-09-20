package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewRollbackApplyCommandBuilder creates a new rollback_apply command
func NewRollbackApplyCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("apply")
	builder.WithShort("Apply a rollback point (restore object states from snapshot)")
	help := clipkg.DynamicHelpBuilder("Apply a rollback point (restore object states from snapshot)")
	help.WithDescriptionLines("Apply a stored rollback point: restore object states from the snapshot to storage (quick path).")
	help.WithDescriptionLines("The rollback point must exist in the project's rollback store.")
	help.AddExample("Apply a rollback point", "%s rollback apply rb-20260102-120000.000000000")
	help.AddExample("Apply with JSON output", "%s rollback apply rb-20260102-120000.000000000 --format json")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.ExactArgs(1))
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
