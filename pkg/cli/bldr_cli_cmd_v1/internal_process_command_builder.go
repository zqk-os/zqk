package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewInternalProcessCommandBuilder creates a new internal_process command
func NewInternalProcessCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("process")
	builder.WithShort("Process a reference file with operations or data (admin only)")
	help := clipkg.DynamicHelpBuilder("Process a reference file with operations or data (admin only)")
	help.WithDescriptionLines("Process a reference file that contains operations or data to be automatically executed.")
	help.AddExample("Process operations from a reference file", "%s internal process --file operations.yaml")
	help.AddExample("Dry-run to see what would be executed", "%s internal process --file operations.yaml --dry-run")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.NoArgs)
	builder.AddBoolFlag("ignore-scheduler-down", "", false, "Ignore scheduler down warning/lockdown")
	builder.AddStringFlag("file", "", "", "Path to reference file (required)")
	builder.AddBoolFlag("dry-run", "", false, "Show what would be executed without actually executing")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
