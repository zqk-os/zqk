package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemFeatureFlagsCommandBuilder creates a new system_feature_flags command
func NewSystemFeatureFlagsCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for feature-flags")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
