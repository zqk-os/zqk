package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemGenerateLifecycleIdListCommandBuilder creates a new system_generate_lifecycle_id_list command
func NewSystemGenerateLifecycleIdListCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for generate-lifecycle-id-list")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
