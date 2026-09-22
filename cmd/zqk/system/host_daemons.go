package system

import (
	"os"

	"github.com/zqk-os/zqk/cmd/zqk/ambient"
	pkgambient "github.com/zqk-os/zqk/pkg/ambient"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/scheduler/hostservice"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

const (
	hostDaemonIDScheduler = "scheduler"
	hostDaemonIDAmbient   = "ambient"
)

type hostDaemonUnit struct {
	ID       string
	Optional bool
	Enabled  func(string) bool
	Start    func(string) error
	Stop     func(string) error
}

type hostDaemonReport struct {
	Started []string
	Skipped []string
	Stopped []string
	Errors  map[string]error
}

func productCLI(projectRoot string) string {
	if exe := paths.ResolveProductCLI(projectRoot); exe != "" {
		return exe
	}
	self, err := os.Executable()
	if err != nil || self == "" {
		return paths.ResolveProductCLI(projectRoot)
	}
	return self
}

func schedulerServiceOutput(exe, verb, projectRoot string) ([]byte, error) {
	cmd := execwrap.Command(exe, "scheduler", "service", verb, "--root", projectRoot)
	cmd.Env = append(os.Environ(), zqkenv.APIKey().Name()+"="+pkgctx.SystemAccountID)
	return cmd.CombinedOutput()
}

func defaultHostDaemons(projectRoot string, logger logging.Logger) []hostDaemonUnit {
	exe := productCLI(projectRoot)
	return []hostDaemonUnit{
		{
			ID: hostDaemonIDScheduler,
			Start: func(root string) error {
				if _, err := hostservice.ResolveEntry(root); err != nil {
					_, _ = schedulerServiceOutput(exe, "install", root)
				}
				_, err := schedulerServiceOutput(exe, "start", root)
				return err
			},
			Stop: func(root string) error {
				_, err := schedulerServiceOutput(exe, "stop", root)
				return err
			},
		},
		{
			ID:       hostDaemonIDAmbient,
			Optional: true,
			Enabled:  pkgambient.HostDaemonEnabled,
			Start: func(root string) error {
				return ambient.EnsureDaemon(root, logger)
			},
			Stop: func(root string) error {
				return ambient.StopDaemon(root)
			},
		},
	}
}

func logHostDaemonReport(logger logging.Logger, action string, report hostDaemonReport) {
	if logger == nil {
		return
	}
	for _, id := range report.Started {
		logging.Fluent(logger).Info("Host daemon started").
			String("daemon", id).
			String("action", action).
			Log()
	}
	for _, id := range report.Skipped {
		logging.Fluent(logger).Info("Host daemon skipped (opt-in off)").
			String("daemon", id).
			String("action", action).
			Log()
	}
	for _, id := range report.Stopped {
		logging.Fluent(logger).Info("Host daemon stopped").
			String("daemon", id).
			String("action", action).
			Log()
	}
	for id, err := range report.Errors {
		logging.Fluent(logger).Warn("Host daemon error").
			String("daemon", id).
			String("action", action).
			String("error", err.Error()).
			Log()
	}
}
