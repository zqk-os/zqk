package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemMigrateStreamSegmentsCommandBuilder creates a new system_migrate_stream_segments command
func NewSystemMigrateStreamSegmentsCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for migrate-stream-segments")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
