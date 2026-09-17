package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemGenerateLifecycleIdListCommandBuilder creates a new system_generate_lifecycle_id_list command
func NewSystemGenerateLifecycleIdListCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for generate-lifecycle-id-list")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
