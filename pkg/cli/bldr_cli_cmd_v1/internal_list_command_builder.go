package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewInternalListCommandBuilder creates a new internal_list command
func NewInternalListCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("list")
	builder.WithShort("List internal or built-in objects")
	help := clipkg.DynamicHelpBuilder("List internal or built-in objects")
	help.WithDescriptionLines("List internal or built-in objects.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("By default, shows only built-in or internal objects (excludes regular public objects).")
	help.WithDescriptionLines("Use --built-in to filter for only built-in instances.")
	help.WithDescriptionLines("Use --internal to filter for only internal objects (visibility: internal).")
	help.WithDescriptionLines("Use --all to show all objects including regular public ones.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("If no kind is specified, lists all internal/built-in objects across all kinds.")
	help.AddExample("List all built-in and internal objects (across all kinds)", "%s internal list")
	help.AddExample("List only built-in objects (across all kinds)", "%s internal list --built-in")
	help.AddExample("List built-in and internal components only", "%s internal list component")
	help.AddExample("List only built-in component types", "%s internal list component --built-in")
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
