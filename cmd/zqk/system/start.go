package system

import (
	"os"
	"path/filepath"

	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/scheduler/hostservice"
	"github.com/zqk-os/zqk/pkg/zqkenv"
	"github.com/spf13/cobra"
)

func NewStartCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemStartCommandBuilder(), &cobra.Command{
		Run: func(cmd *cobra.Command, args []string) {
			logger := logging.GetLogger()
			logging.FluentEvent(logger).Info("Initiating system daemon start...").Log()

			exe, _ := os.Executable()
			if exe == "" {
				exe = "zqk"
			}

			absRoot, err := filepath.Abs(".")
			if err != nil {
				absRoot = "."
			}

			// Ensure scheduler host service is installed for this project root
			if _, err := hostservice.ResolveEntry(absRoot); err != nil {
				logging.FluentEvent(logger).Info("Host service not registered; installing scheduler service...").
					String("root", absRoot).
					Log()
				installCmd := execwrap.Command(exe, "scheduler", "service", "install", "--root", absRoot)
				installCmd.Env = append(os.Environ(), zqkenv.APIKey().Name()+"="+pkgctx.SystemAccountID)
				if out, err := installCmd.CombinedOutput(); err != nil {
					logging.FluentEvent(logger).Warn("Could not auto-install scheduler service").
						String("error", err.Error()).
						String("output", string(out)).
						Log()
				}
			}

			logging.FluentEvent(logger).Info("Starting scheduler daemon...").String("root", absRoot).Log()
			execCmd := execwrap.Command(exe, "scheduler", "service", "start", "--root", absRoot)
			execCmd.Env = append(os.Environ(), zqkenv.APIKey().Name()+"="+pkgctx.SystemAccountID)
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
