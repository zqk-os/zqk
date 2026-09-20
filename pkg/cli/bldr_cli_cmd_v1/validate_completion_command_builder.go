package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewValidateCompletionCommandBuilder creates a new validate_completion command
func NewValidateCompletionCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for completion")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
