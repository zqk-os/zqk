package inbox

import (
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/spf13/cobra"
)

// NewInboxCmd creates the inbox command group for managing the Autonomy Inbox.
func NewInboxCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewInboxCommandBuilder()
	cmd.AddCommand(NewListCmd())
	return cmd
}
