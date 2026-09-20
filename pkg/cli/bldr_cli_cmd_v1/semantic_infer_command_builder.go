package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSemanticInferCommandBuilder creates a new semantic_infer command
func NewSemanticInferCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("infer")
	builder.WithShort("infer command")
	help := clipkg.DynamicHelpBuilder("infer command")
	help.WithDescriptionLines("infer command")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
