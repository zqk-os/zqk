package bldr_cli_cmd_v1

import (
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewOrganizationalPropagateCommandBuilder creates a new organizational_propagate command
func NewOrganizationalPropagateCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("propagate")
	builder.WithShort("Propagate organizational change to affected ZQK objects")
	help := clipkg.DynamicHelpBuilder("Propagate organizational change to affected ZQK objects")
	help.WithDescriptionLines("Propagate an organizational change to affected ZQK objects.")
	help.WithDescriptionLines("Requires an impact_analysis for the change (run \"organizational analyze-impact --change <id>\" first).")
	help.WithDescriptionLines("With --confirm, updates each affected object with last_propagated_change_ref so propagation is recorded.")
	help.AddExample("Dry run: show what would be propagated", "%s organizational propagate --change OCH-001 --dry-run")
	help.AddExample("Apply propagation (updates affected objects)", "%s organizational propagate --change OCH-001 --confirm")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.AddStringFlag("change", "", "", "Organizational change object ID (required)")
	builder.AddBoolFlag("confirm", "", false, "Apply propagation (update affected objects)")
	builder.AddBoolFlag("dry-run", "", false, "Show what would be propagated without updating")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
