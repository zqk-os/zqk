package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemSeedQuestionsCommandBuilder creates a new system_seed_questions command
func NewSystemSeedQuestionsCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for seed-questions")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
