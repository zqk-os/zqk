package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemGenerateLifecycleBuildersCommandBuilder creates a new system_generate_lifecycle_builders command
func NewSystemGenerateLifecycleBuildersCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for generate-lifecycle-builders")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
