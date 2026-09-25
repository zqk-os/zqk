package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewNewSwarmCommandBuilder creates a new new_swarm command
func NewNewSwarmCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("swarm")
	builder.WithShort("Write a portable swarm manifest draft (swarm.yaml)")
	help := clipkg.DynamicHelpBuilder("Write a portable swarm manifest draft (swarm.yaml)")
	help.WithDescriptionLines("Valid holonic swarm package manifest conforming to swarm_package_spec.")
	help.WithDescriptionLines("Next: zqk pack seal <dir> or zqk run <dir>")
	help.AddExample("Minimal swarm manifest draft", "%s new swarm")
	help.AddExample("Named swarm manifest", "%s new swarm --name code-migration --description \"Migrate packages to Go 1.24\"")
	help.AddExample("Cellular archetype swarm", "%s new swarm --name analysis-cell --cell-type neuron")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.NoArgs)
	builder.AddStringFlag("output", "o", "", "Output file path, or '-' for stdout (default: .zqk/drafts/swarm-<timestamp>.yaml)")
	builder.AddStringFlag("name", "", "custom-swarm", "Package name for the swarm")
	builder.AddStringFlag("version", "", "1.0.0", "SemVer version for the swarm package")
	builder.AddStringFlag("description", "", "", "Package description (empty uses default description)")
	builder.AddStringFlag("author", "", "zqk-community", "Author or owner of the swarm package")
	builder.AddStringFlag("license", "", "Apache-2.0", "License identifier for the swarm package")
	builder.AddStringFlag("cell-type", "", "", "Cellular team archetype (neuron, muscle, heart, lungs)")
	builder.AddStringFlag("goal-metric", "", "", "Target metric name for the swarm goal")
	builder.AddStringFlag("goal-target", "", "", "Target goal value/percentage (e.g. 95%)")
	builder.AddStringFlag("goal-description", "", "", "Human-readable goal description")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	cmd.Aliases = []string{"swarm-manifest", "swarm-spec", "pack-manifest"}
	return cmd
}
