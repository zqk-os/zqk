package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemGenerateProfileBuildersCommandBuilder creates a new system_generate_profile_builders command
func NewSystemGenerateProfileBuildersCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for generate-profile-builders")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
