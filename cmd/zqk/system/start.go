package system

import (
	"os"
	"path/filepath"
	"runtime"

	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/spf13/cobra"
)

func NewStartCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemStartCommandBuilder(), &cobra.Command{
		Run: func(cmd *cobra.Command, args []string) {
			logger := logging.GetLogger()
			logging.FluentEvent(logger).Info("Initiating system-wide daemon start...").Log()

			exe, _ := os.Executable()
			if exe == "" {
				exe = "zqk"
			}

			if runtime.GOOS == "darwin" {
				logging.FluentEvent(logger).Info("Loading macOS LaunchAgents (com.zqk.*)...").Log()
				home, _ := os.UserHomeDir()
				if home != "" {
					matches, _ := filepath.Glob(filepath.Join(home, "Library", "LaunchAgents", "com.zqk.*.plist"))
					for _, plist := range matches {
						logging.FluentEvent(logger).Debug("Loading agent").String("plist", filepath.Base(plist)).Log()
						_ = execwrap.Command("launchctl", "load", plist).Run()
					}
				}
			} else if runtime.GOOS == "linux" {
				logging.FluentEvent(logger).Info("Starting systemd user services (zqk-*)...").Log()
				_ = execwrap.Command("sh", "-c", "systemctl --user start zqk-*").Run()
			}

			logging.FluentEvent(logger).Info("Starting scheduler daemon...").Log()
			execCmd := execwrap.Command(exe, "scheduler", "service", "start")
			out, err := execCmd.CombinedOutput()
			if err != nil {
				logging.FluentEvent(logger).Error("Failed to start scheduler", err).
					String("error", err.Error()).
					String("output", string(out)).
					Log()
			} else {
				logging.FluentEvent(logger).Info("Scheduler daemon started.").Log()
			}

			logging.FluentEvent(logger).Info("System start complete.").Log()
		},
	})
	return cmd
}
