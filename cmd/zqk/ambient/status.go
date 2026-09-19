// Traceability: BLI-SYM-008, BLI-SYM-010, REQ-SYM-005
package ambient

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/objects"
)

func newStatusCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewAmbientStatusCommandBuilder()
	cmd.RunE = runStatus
	return cmd
}

func runStatus(cmd *cobra.Command, args []string) error {
	projectRoot := cli.ResolveProjectRoot(".")
	if pid, running := readAmbientPID(projectRoot); running {
		return cli.FormatOutput(cmd, map[string]any{
			objects.FieldKeyComponent: "ambient",
			objects.FieldKeyStatus:    "running",
			"pid":                      pid,
			"project_root":             projectRoot,
		})
	}

	return cli.FormatOutput(cmd, map[string]any{
		objects.FieldKeyComponent: "ambient",
		objects.FieldKeyStatus:    "stopped",
	})
}
