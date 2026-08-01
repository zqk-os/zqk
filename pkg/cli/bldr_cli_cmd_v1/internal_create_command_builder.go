package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewInternalCreateCommandBuilder creates a new internal_create command
func NewInternalCreateCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("create")
	builder.WithShort("Create an internal object (admin only)")
	help := clipkg.DynamicHelpBuilder("Create an internal object (admin only)")
	help.WithDescriptionLines("Create an internal object with admin privileges.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("This command allows creating objects that may be internal or built-in instances,")
	help.WithDescriptionLines("including object specifications, lifecycles, templates, and other system objects.")
	help.AddExample("Create an object specification", "%s internal create object_spec --file my_spec.yaml")
	help.AddExample("Create an internal lifecycle definition", "%s internal create lifecycle --file lifecycle.yaml")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.ExactArgs(1))
	builder.AddBoolFlag("ignore-scheduler-down", "", false, "Ignore scheduler down warning/lockdown")
	builder.AddStringFlag("file", "", "", "Path to YAML file containing object data (required)")
	builder.AddStringFlag("data", "", "", "Inline YAML data for the object")
	builder.AddBoolFlag("dry-run", "", false, "Show what would be created without actually creating it")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
