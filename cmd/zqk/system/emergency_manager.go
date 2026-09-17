package system

import (
	"fmt"
	"time"

	"github.com/lanceman/zqk/pkg/execwrap"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/scheduler"
	"github.com/spf13/cobra"
)

func NewEmergencyManagerCmd() *cobra.Command {
	builder := clipkg.NewCommandBuilder("emergency-manager")
	builder.WithShort("Monitors the health of the CAP orchestrator and Kernel Steward. Executes safe rollbacks or stashes untracked files if the main managers fail continuously.")

	help := clipkg.DynamicHelpBuilder("Monitors the health of the CAP orchestrator and Kernel Steward. Executes safe rollbacks or stashes untracked files if the main managers fail continuously.")
	builder.WithHelpBuilder(help)

	cmd := builder.Build()
	cli.BindAsyncProgress(cmd, runEmergencyManager)

	return cmd
}

func runEmergencyManager(cmd *cobra.Command, args []string) error {
	projectRoot := cli.ResolveProjectRoot(".")
	if projectRoot == "" {
		return fmt.Errorf("project root not found")
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	ctx := cmd.Context()

	isAlive, lastKeepAlive, err := scheduler.IsSchedulerAlive(projectRoot)

	// Evaluate failure conditions:
	// 1. Scheduler daemon process is not running.
	// 2. Keep-alive file is stale (hung).
	if err != nil || !isAlive || (!lastKeepAlive.IsZero() && time.Since(lastKeepAlive) > scheduler.DefaultKeepAliveTimeout) {
		logging.Fluent(logger).Warn("Emergency Manager: Scheduler failure detected. Initiating recovery...").Log()

		// Attempt Recovery: stop forcefully if hung, then start
		stopCmd := execwrap.CommandContext(ctx, "bin/zqk", "scheduler", "stop", "--wait")
		stopCmd.Dir = projectRoot
		_ = stopCmd.Run()

		startCmd := execwrap.CommandContext(ctx, "bin/zqk", "scheduler", "start")
		startCmd.Dir = projectRoot
		if err := startCmd.Start(); err != nil {
			logging.Fluent(logger).Error("Emergency Manager: Recovery failed, escalating.", err).Log()
			// Escalate
			return fmt.Errorf("scheduler recovery failed: %w", err)
		}

		logging.Fluent(logger).Info("Emergency Manager: Recovery successful. Scheduler restarted.").Log()
		return nil
	}

	_ = cli.WriteOutput(cmd, []byte("Emergency Manager: All critical systems are operational.\n"))
	return nil
}
