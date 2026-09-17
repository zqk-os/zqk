package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemFeatureFlagsCommandBuilder creates a new system_feature_flags command
func NewSystemFeatureFlagsCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for feature-flags")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
