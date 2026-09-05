package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemAnalyzeCommandBuilder creates a new system_analyze command
func NewSystemAnalyzeCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for analyze")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
