package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemPrepareOnboardingCommandBuilder creates a new system_prepare_onboarding command
func NewSystemPrepareOnboardingCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for prepare-onboarding")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
