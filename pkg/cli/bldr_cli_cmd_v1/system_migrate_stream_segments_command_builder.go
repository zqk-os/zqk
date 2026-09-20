package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemMigrateStreamSegmentsCommandBuilder creates a new system_migrate_stream_segments command
func NewSystemMigrateStreamSegmentsCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for migrate-stream-segments")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
