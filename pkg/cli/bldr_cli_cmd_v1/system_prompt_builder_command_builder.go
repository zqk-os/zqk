package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemPromptBuilderCommandBuilder creates a new system_prompt_builder command
func NewSystemPromptBuilderCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for prompt-builder")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
