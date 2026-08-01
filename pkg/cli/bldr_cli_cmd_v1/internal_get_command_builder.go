package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewInternalGetCommandBuilder creates a new internal_get command
func NewInternalGetCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("get")
	builder.WithShort("Get an internal or built-in object by ID")
	help := clipkg.DynamicHelpBuilder("Get an internal or built-in object by ID")
	help.WithDescriptionLines("Get an internal or built-in object by ID.")
	help.AddExample("Get a built-in object", "%s internal get COMP-TYPE-001")
	help.AddExample("Get an internal object", "%s internal get LIFECYCLE-001")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.ExactArgs(1))
	builder.AddBoolFlag("ignore-scheduler-down", "", false, "Ignore scheduler down warning/lockdown")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
