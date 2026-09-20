package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemPromptBuilderCommandBuilder creates a new system_prompt_builder command
func NewSystemPromptBuilderCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for prompt-builder")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
