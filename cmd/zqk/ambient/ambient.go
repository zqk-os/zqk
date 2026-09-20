// Traceability: BLI-SYM-008, BLI-SYM-010, REQ-SYM-005
package ambient

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
)

// NewAmbientCmd returns the root ambient command.
func NewAmbientCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewAmbientCommandBuilder()
	cmd.AddCommand(newStatusCmd())
	cmd.AddCommand(newAutomergeCmd())
	cmd.AddCommand(newIngestCmd())
	cmd.AddCommand(newWaveCmd())
	cmd.AddCommand(newDaemonCmd())
	cmd.AddCommand(newStartCmd())
	cmd.AddCommand(newStopCmd())
	cmd.AddCommand(newEnsureCmd())
	return cmd
}
