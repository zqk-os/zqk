package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemGenerateApiBuildersCommandBuilder creates a new system_generate_api_builders command
func NewSystemGenerateApiBuildersCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for generate-api-builders")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
