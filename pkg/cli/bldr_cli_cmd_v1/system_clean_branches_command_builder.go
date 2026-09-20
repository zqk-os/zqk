package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemCleanBranchesCommandBuilder creates a new system_clean_branches command
func NewSystemCleanBranchesCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for clean-branches")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
