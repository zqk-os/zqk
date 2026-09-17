package system

import (
	"os"
	"path/filepath"
	"runtime"

	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/execwrap"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/spf13/cobra"
)

func NewShutdownCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemShutdownCommandBuilder(), &cobra.Command{
		Run: func(cmd *cobra.Command, args []string) {
			logger := logging.GetLogger()
			logging.FluentEvent(logger).Info("Initiating system-wide daemon shutdown...").Log()

			exe, _ := os.Executable()
			if exe == "" {
				exe = "zqk"
			}

			logging.FluentEvent(logger).Info("Stopping scheduler daemon (via host service manager)...").Log()
			execCmd := execwrap.Command(exe, "scheduler", "service", "stop")
			out, err := execCmd.CombinedOutput()
			if err != nil {
				logging.FluentEvent(logger).Warn("Scheduler daemon may not be running or failed to stop").
					String("error", err.Error()).
					String("output", string(out)).
					Log()
			} else {
				logging.FluentEvent(logger).Info("Scheduler daemon stopped.").Log()
			}

			if runtime.GOOS == "darwin" {
				logging.FluentEvent(logger).Info("Unloading macOS LaunchAgents (com.zqk.*)...").Log()
				home, _ := os.UserHomeDir()
				if home != "" {
					matches, _ := filepath.Glob(filepath.Join(home, "Library", "LaunchAgents", "com.zqk.*.plist"))
					for _, plist := range matches {
						logging.FluentEvent(logger).Debug("Unloading agent").String("plist", filepath.Base(plist)).Log()
						_ = execwrap.Command("launchctl", "unload", plist).Run()
					}
				}
			} else if runtime.GOOS == "linux" {
				logging.FluentEvent(logger).Info("Stopping systemd user services (zqk-*)...").Log()
				_ = execwrap.Command("sh", "-c", "systemctl --user stop zqk-*").Run()
			}

			logging.FluentEvent(logger).Info("Terminating any stray ZQK background services...").Log()
			_ = execwrap.Command("pkill", "-f", "zqk-mcp").Run()
			_ = execwrap.Command("pkill", "-f", "zqk object daemon").Run()
			_ = execwrap.Command("pkill", "-f", "zqk scheduler start").Run()

			logging.FluentEvent(logger).Info("System shutdown complete.").Log()
		},
	})
	return cmd
}
