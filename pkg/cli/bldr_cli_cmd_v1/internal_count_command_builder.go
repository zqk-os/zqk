package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewInternalCountCommandBuilder creates a new internal_count command
func NewInternalCountCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("count")
	builder.WithShort("Count internal or built-in objects by kind")
	help := clipkg.DynamicHelpBuilder("Count internal or built-in objects by kind")
	help.WithDescriptionLines("Count internal or built-in objects by kind with optional filtering.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("When no kind is specified, counts all internal/built-in object kinds.")
	help.AddExample("Count all internal/built-in objects", "%s internal count")
	help.AddExample("Count objects of a specific kind", "%s internal count component")
	help.AddExample("Count only built-in objects", "%s internal count --built-in")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.MaximumNArgs(1))
	builder.AddBoolFlag("ignore-scheduler-down", "", false, "Ignore scheduler down warning/lockdown")
	builder.AddBoolFlag("built-in", "", false, "Filter for built-in instances only")
	builder.AddBoolFlag("internal", "", false, "Filter for internal objects (visibility: internal) only")
	builder.AddBoolFlag("all", "", false, "Show all objects (including regular public objects)")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
