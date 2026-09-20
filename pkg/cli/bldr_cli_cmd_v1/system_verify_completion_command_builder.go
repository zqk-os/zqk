package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemVerifyCompletionCommandBuilder creates a new system_verify_completion command
func NewSystemVerifyCompletionCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for verify-completion")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
