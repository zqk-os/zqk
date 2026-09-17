package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemDetectSpecChangesCommandBuilder creates a new system_detect_spec_changes command
func NewSystemDetectSpecChangesCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for detect-spec-changes")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
