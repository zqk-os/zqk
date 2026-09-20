package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemGenerateApiBuildersCommandBuilder creates a new system_generate_api_builders command
func NewSystemGenerateApiBuildersCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for generate-api-builders")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
