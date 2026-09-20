package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewUtilityFixRegistrationCommandBuilder creates a new utility_fix_registration command
func NewUtilityFixRegistrationCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for fix-registration")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
