package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewNewBundleCommandBuilder creates a new new_bundle command
func NewNewBundleCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("bundle")
	builder.WithShort("Write a minimal scenario bundle draft")
	help := clipkg.DynamicHelpBuilder("Write a minimal scenario bundle draft")
	help.WithDescriptionLines("Valid scenario_bundle document; add objects.goals, objects.requirements, etc. as needed.")
	help.WithDescriptionLines("Next: zqk-scenario bundle apply -f <draft> -R .")
	help.AddExample("Minimal bundle draft", "%s new bundle")
	help.AddExample("Named bundle metadata", "%s new bundle --name my-traceability-bundle --description \"Stamp REQ/criteria\"")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.NoArgs)
	builder.AddStringFlag("output", "o", "", "Output file path, or '-' for stdout (default: .zqk/drafts/bundle-<timestamp>.yaml)")
	builder.AddStringFlag("name", "", "draft-bundle", "metadata.name for the bundle")
	builder.AddStringFlag("description", "", "", "metadata.description (empty uses default help text)")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
