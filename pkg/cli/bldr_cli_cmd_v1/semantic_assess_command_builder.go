package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSemanticAssessCommandBuilder creates a new semantic_assess command
func NewSemanticAssessCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for assess")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
