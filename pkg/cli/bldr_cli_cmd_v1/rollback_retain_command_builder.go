package bldr_cli_cmd_v1

import (
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewRollbackRetainCommandBuilder creates a new rollback_retain command
func NewRollbackRetainCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("retain")
	builder.WithShort("Trim rollback store to configured keep count and duration")
	help := clipkg.DynamicHelpBuilder("Trim rollback store to configured keep count and duration")
	help.WithDescriptionLines("Trims the rollback point store to the configured retain count and duration")
	help.WithDescriptionLines("(ZQK_ROLLBACK_RETAIN_COUNT, ZQK_ROLLBACK_RETAIN_DURATION). Call periodically")
	help.WithDescriptionLines("or after capture to keep storage bounded.")
	help.AddExample("Trim rollback store to config", "%s rollback retain")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
