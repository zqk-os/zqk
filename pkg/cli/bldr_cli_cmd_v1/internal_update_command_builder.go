package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewInternalUpdateCommandBuilder creates a new internal_update command
func NewInternalUpdateCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("update")
	builder.WithShort("Update an internal or built-in object (admin only)")
	help := clipkg.DynamicHelpBuilder("Update an internal or built-in object (admin only)")
	help.WithDescriptionLines("Update an internal or built-in object with admin privileges.")
	help.AddExample("Update a field", "%s internal update COMP-TYPE-001 --field title=\"Updated Title\"")
	help.AddExample("Update from file", "%s internal update COMP-TYPE-001 --file updated.yaml")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.ExactArgs(1))
	builder.AddBoolFlag("ignore-scheduler-down", "", false, "Ignore scheduler down warning/lockdown")
	builder.AddStringArrayFlag("field", "", "Update a field (field=value). Repeatable.")
	builder.AddStringFlag("file", "", "", "Path to YAML file containing update data")
	builder.AddStringFlag("data", "", "", "Inline YAML data for updates")
	builder.AddBoolFlag("auto-status", "", false, "Advance status to the next lifecycle-valid status for this object kind")
	builder.AddBoolFlag("dry-run", "", false, "Show what would be updated without actually updating")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
