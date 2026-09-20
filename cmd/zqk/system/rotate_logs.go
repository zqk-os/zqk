package system

import (
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/scheduler"
)

func NewRotateLogsCmd() *cobra.Command {
	return clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemRotateLogsCommandBuilder(), &cobra.Command{
		Use:   "rotate-logs",
		Short: "Rotate and cleanup logs",
		RunE: func(cmd *cobra.Command, args []string) error {
			rotator := scheduler.NewLogRotator(".zqk/logs", logging.GetLoggerFromProfile("system"))
			err := rotator.Rotate(cmd.Context(), 7*24*time.Hour)
			if err == nil {
				return cli.WriteOutput(cmd, []byte("Logs rotated successfully\n"))
			}
			return err
		},
	})
}
