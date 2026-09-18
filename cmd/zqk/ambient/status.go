// Traceability: BLI-SYM-008, BLI-SYM-010, REQ-SYM-005
package ambient

import (
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/ambient"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewAmbientStatusCommandBuilder()
	cmd.RunE = runStatus
	return cmd
}

func runStatus(cmd *cobra.Command, args []string) error {
	hub := ambient.NewEventHub()
	status := hub.Status()

	return cli.FormatOutput(cmd, map[string]any{
		objects.FieldKeyComponent: "ambient",
		objects.FieldKeyStatus:    status,
	})
}
