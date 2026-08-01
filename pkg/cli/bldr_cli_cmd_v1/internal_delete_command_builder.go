package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewInternalDeleteCommandBuilder creates a new internal_delete command
func NewInternalDeleteCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("delete")
	builder.WithShort("Delete an internal or built-in object (admin only)")
	help := clipkg.DynamicHelpBuilder("Delete an internal or built-in object (admin only)")
	help.WithDescriptionLines("Delete an internal or built-in object with admin privileges.")
	help.AddExample("Delete a built-in object (dangerous)", "%s internal delete COMP-TYPE-001")
	help.AddExample("Delete with cascade", "%s internal delete COMP-TYPE-001 --cascade")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.ExactArgs(1))
	builder.AddBoolFlag("ignore-scheduler-down", "", false, "Ignore scheduler down warning/lockdown")
	builder.AddBoolFlag("cascade", "", false, "Delete object and all objects that reference it")
	builder.AddBoolFlag("unlink-references", "", false, "Strip this ID from dependents' reference fields, then delete (does not delete dependent objects)")
	builder.AddBoolFlag("dry-run", "", false, "Show what would be deleted without actually deleting it")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
