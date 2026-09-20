package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemDetectSpecChangesCommandBuilder creates a new system_detect_spec_changes command
func NewSystemDetectSpecChangesCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for detect-spec-changes")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
