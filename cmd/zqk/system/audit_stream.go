package system

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/lanceman/zqk/pkg/audit"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/spf13/cobra"
)

// NewAuditStreamCmd returns a command to tail the audit stream.
func NewAuditStreamCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemAuditStreamCommandBuilder(), &cobra.Command{
		Use:   "audit-stream",
		Short: "Stream real-time audit events",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background() // Background: request-or-shutdown derived

			// For demonstration of the in-memory Event Bus stream pattern:
			// (In production, this would tail a JSONL WAL file or connect to daemon RPC)
			stream := audit.NewAuditStream()

			ch := stream.Subscribe(ctx)

			// A real CLI would connect to the daemon and receive events over IPC.
			// For now, it blocks waiting on the stream.
			for record := range ch {
				b, err := json.Marshal(record)
				if err == nil {
					fmt.Fprintln(cmd.OutOrStdout(), string(b))
				}
			}
			return nil
		},
	})
	return cmd
}
