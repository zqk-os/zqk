package system

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mitchellh/go-ps"
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/brand"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/process"
	"github.com/zqk-os/zqk/pkg/service"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func NewShutdownCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemShutdownCommandBuilder(), &cobra.Command{
		Run: func(cmd *cobra.Command, args []string) {
			logger := logging.GetLogger()
			logging.FluentEvent(logger).Info("Initiating system daemon shutdown...").Log()

			// Pluggable host service manager check
			hostMgr := service.NewManager()
			if hostMgr != nil && hostMgr.Adapter() != nil {
				logging.FluentEvent(logger).Info("Host service adapter active for shutdown").
					String("adapter", hostMgr.Adapter().Name()).
					Log()
			}

			exe, _ := os.Executable()
			if exe == "" {
				exe = brand.ExecutableName()
			}

			absRoot, err := filepath.Abs(".")
			if err != nil {
				absRoot = "."
			}

			logging.FluentEvent(logger).Info("Stopping scheduler daemon (via host service manager)...").
				String("root", absRoot).
				Log()
			execCmd := execwrap.Command(exe, "scheduler", "service", "stop", "--root", absRoot)
			execCmd.Env = append(os.Environ(), zqkenv.APIKey().Name()+"="+pkgctx.SystemAccountID)
			out, err := execCmd.CombinedOutput()
			if err != nil {
				logging.FluentEvent(logger).Warn("Scheduler daemon may not be running or failed to stop").
					String("error", err.Error()).
					String("output", string(out)).
					Log()
			} else {
				logging.FluentEvent(logger).Info("Scheduler daemon stopped.").Log()
			}

			logging.FluentEvent(logger).Info("Terminating any stray background services for this project...").Log()
			if procs, err := ps.Processes(); err == nil {
				curPid := os.Getpid()
				for _, p := range procs {
					pid := p.Pid()
					if pid <= 1 || pid == curPid {
						continue
					}
					base := filepath.Base(p.Executable())
					if !brand.IsProductExecutable(base) {
						continue
					}
					cmdLine := process.ProcessCommandLine(pid)
					if strings.Contains(cmdLine, "scheduler start") && strings.Contains(cmdLine, absRoot) {
						if proc, err := os.FindProcess(pid); err == nil {
							_ = proc.Kill()
						}
					}
				}
			}

			logging.FluentEvent(logger).Info("System shutdown complete.").Log()
		},
	})
	return cmd
}
