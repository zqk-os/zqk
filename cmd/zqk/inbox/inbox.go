package inbox

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
)

// NewInboxCmd creates the inbox command group for managing the Autonomy Inbox.
func NewInboxCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewInboxCommandBuilder()
	cmd.AddCommand(NewListCmd())
	return cmd
}
