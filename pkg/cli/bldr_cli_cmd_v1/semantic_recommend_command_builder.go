package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSemanticRecommendCommandBuilder creates a new semantic_recommend command
func NewSemanticRecommendCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for recommend")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
