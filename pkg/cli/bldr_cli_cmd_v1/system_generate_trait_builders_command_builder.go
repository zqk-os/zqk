package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemGenerateTraitBuildersCommandBuilder creates a new system_generate_trait_builders command
func NewSystemGenerateTraitBuildersCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for generate-trait-builders")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
