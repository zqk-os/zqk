package bldr_cli_cmd_v1

import (
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewRollbackListCommandBuilder creates a new rollback_list command
func NewRollbackListCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("list")
	builder.WithShort("List rollback points (most recent last)")
	help := clipkg.DynamicHelpBuilder("List rollback points (most recent last)")
	help.WithDescriptionLines("List rollback points for the project. Points are ordered with most recent last.")
	help.WithDescriptionLines("Use --last to limit the number of points and --within to filter by time range (e.g. 24h).")
	help.AddExample("List all rollback points", "%s rollback list")
	help.AddExample("List last 10 points", "%s rollback list --last 10")
	help.AddExample("List points within 24 hours", "%s rollback list --within 24h")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.AddIntFlag("last", "", 0, "Limit to last N points (0 = all)")
	builder.AddStringFlag("within", "", "", "Only points within this duration (e.g. 24h)")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
