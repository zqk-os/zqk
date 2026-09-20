package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemGenerateSpecIndexCommandBuilder creates a new system_generate_spec_index command
func NewSystemGenerateSpecIndexCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for generate-spec-index")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
