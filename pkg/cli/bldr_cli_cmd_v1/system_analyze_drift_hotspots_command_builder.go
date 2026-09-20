package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemAnalyzeDriftHotspotsCommandBuilder creates a new system_analyze_drift_hotspots command
func NewSystemAnalyzeDriftHotspotsCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for analyze-drift-hotspots")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
