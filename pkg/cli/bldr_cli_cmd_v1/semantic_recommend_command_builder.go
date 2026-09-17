package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSemanticRecommendCommandBuilder creates a new semantic_recommend command
func NewSemanticRecommendCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for recommend")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
