package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewObjectWstransShowCommandBuilder creates a new object_wstrans_show command
func NewObjectWstransShowCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("show")
	builder.WithShort("Show a workstream transition by ID or the first transition")
	help := clipkg.DynamicHelpBuilder("Show a workstream transition by ID or the first transition")
	help.WithDescriptionLines("Display a workstream transition. If id is provided, show that transition; otherwise list")
	help.WithDescriptionLines("workstream_transition and show the first.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Respects field-level read permissions (BLI-642). Output format via --format.")
	help.AddExample("Show first workstream transition", "%s object wstrans show")
	help.AddExample("Show by ID", "%s object wstrans show WST-001")
	help.AddExample("Show in JSON format", "%s object wstrans show --format json")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.MaximumNArgs(1))
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
