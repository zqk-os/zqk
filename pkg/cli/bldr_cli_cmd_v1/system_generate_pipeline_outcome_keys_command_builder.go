package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemGeneratePipelineOutcomeKeysCommandBuilder creates a new system_generate_pipeline_outcome_keys command
func NewSystemGeneratePipelineOutcomeKeysCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for generate-pipeline-outcome-keys")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
