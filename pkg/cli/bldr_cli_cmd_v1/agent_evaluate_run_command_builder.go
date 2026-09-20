package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewAgentEvaluateRunCommandBuilder creates a new agent_evaluate_run command
func NewAgentEvaluateRunCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for evaluate-run")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
