package app

import (
	"fmt"
	"path/filepath"
	"runtime/pprof"
	"time"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	clitool "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/policyinterrupt"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/when"
	"github.com/zqk-os/zqk/pkg/zqkenv"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

func rootPersistentPostRunE(cmd *cobra.Command, args []string) error {
	// Stop CPU profiling and close file if we started it
	if cpuProfileFile != nil {
		pprof.StopCPUProfile()
		_ = cpuProfileFile.Close()
		cpuProfileFile = nil
	}
	// Write goroutine profile if requested
	if goroutineProfilePath != "" {
		_ = writeGoroutineProfile(goroutineProfilePath)
		goroutineProfilePath = ""
	}
	// Skip tracking for help/version commands
	isHelpCommand := cmd.Name() == "help" || cmd.Name() == "version"
	if isHelpCommand {
		return nil
	}

	// Get tracker from context
	cmdCtx := cmd.Context()
	if cmdCtx == nil {
		return nil
	}

	tracker := clitool.GetTrackerFromContext(cmdCtx)
	if tracker == nil {
		return nil
	}

	// Touch session (updated_at, title) only when storage is already available; throttled to avoid WAL storm when many invocations share one session.
	if sessionID := GetZqkSessionIDFromContext(cmdCtx); sessionID != EmptyValue && cli.StorageAvailableForOptionalUse(cmd) {
		projectRoot := cli.ResolveProjectRoot(".")
		if projectRoot != EmptyValue {
			if p := cli.GetStorageProvider(cmd.Context()); p != nil {
				if sp, ok := p.(storage.ObjectStorageProvider); ok {
					accountID := tracker.ActorID
					if accountID == EmptyValue {
						accountID = pkgctx.SystemAccountID
					}
					TouchSessionIfNotThrottled(cmdCtx, projectRoot, sessionID, cmd.CommandPath(), accountID, sp)
				}
			}
		}
	}

	// Get execution result (success/failure)
	// Note: We can't get the actual error here, but we can infer success from the fact
	// that PostRunE was called (if there was an error, RunE would have returned it)
	// For now, we'll set success=true and let the actual error be captured elsewhere
	tracker.SetOutcome(true, 0, nil, false)

	// Convert tracker to metric and record
	metric := tracker.ToCommandMetric()

	// Record metrics if metrics store is available
	hook := clitool.GetTimeoutHook()
	if hook != nil {
		// Get metrics store from hook (it's set in Execute())
		// We need to access it, but it's private. For now, we'll record via the hook's method
		// Actually, we should use the hook's RecordCommandExecution method if available
		// But since we're in PostRunE, the command has already executed
		// The timeout hook's WrapCommand already recorded basic metrics
		// We need to enhance those metrics with the additional context

		// For now, we'll create an audit event with the full tracking information
		// The metrics store update can happen asynchronously
	}

	// Create audit event only when storage is already available (ignorable operation:
	// use existing component, never create — see docs/architecture/COMMAND_ORCHESTRATION.md vital vs ignorable).
	projectRoot := cli.ResolveProjectRoot(".")
	if projectRoot != EmptyValue && cli.StorageAvailableForOptionalUse(cmd) {
		profile := profileHuman
		if c := cli.GetContext(cmd); c != nil && c.Profile != EmptyValue {
			profile = c.Profile
		}
		createCommandAuditEvent(cmd, projectRoot, metric, profile)
	}

	return nil
}

// argsContainHelpOrVersion returns true if any of the given args is a help or version flag.
func argsContainHelpOrVersion(args []string) bool {
	for _, arg := range args {
		switch arg {
		case "help", "--help", "-h", "version", "--version", "-v":
			return true
		}
	}
	return false
}

// checkCLIReminderInRoot checks for CLI reminder and displays it prominently
// This runs in PersistentPreRunE so it appears before every command
func checkCLIReminderInRoot(projectRoot string) {
	if projectRoot == EmptyValue {
		return
	}

	flagFile := filepath.Join(projectRoot, paths.ProjectDataDir, "cli_reminder.flag")
	if info, err := fileutil.Stat(flagFile); err == nil {
		// Only show if file is recent (within last hour) to avoid spam
		if time.Since(info.ModTime()) < time.Hour {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			logging.Fluent(logger).Warn("CLI-FIRST REMINDER (INTERRUPT NOTIFICATION)").Log()

			if content, err := fileutil.ReadFile(flagFile); err == nil {
				logging.Fluent(logger).Info(string(content)).Log()
			}

			modTime := info.ModTime()
			age := time.Since(modTime)
			logging.Fluent(logger).Info("Reminder last updated").
				String("last_updated", zqktime.FormatLayoutUTC(modTime, zqktime.LayoutDateTimeSpace)).
				String("age", formatReminderDuration(age)).
				Log()
		}
	}
}

// checkPolicyInterruptGateInRoot short-circuits CLI flows for non-system actors when a critical policy
// interrupt is pending and unacknowledged. Background/system flows should not be blocked.
// Actor identity is best-effort: prefer explicit env vars; default is system.
func checkPolicyInterruptGateInRoot(projectRoot string) error {
	if projectRoot == EmptyValue {
		return nil
	}
	actorID := zqkenv.AccountID().Get()
	if actorID == EmptyValue {
		actorID = zqkenv.MCPAccountID().Get()
	}
	if actorID == EmptyValue || actorID == pkgctx.SystemAccountID {
		return nil
	}

	acks, err := policyinterrupt.LoadAcksIncremental(projectRoot)
	if err != nil {
		return nil // best-effort gate; do not block on WAL read errors
	}
	latest, err := policyinterrupt.LoadLatestCriticalUnacked(projectRoot, acks)
	if err != nil || latest == nil {
		return nil
	}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	msg := latest.Message
	if msg == EmptyValue {
		msg = "Critical policy interrupt pending"
	}
	logging.Fluent(logger).Warn("CRITICAL POLICY INTERRUPT (ACK REQUIRED)").
		String("message", msg).
		String("dedupe_key", latest.DedupeKey).
		String("policy_id", latest.PolicyID).
		String("action", paths.RewriteCanonicalCLIInvocations("Run: zqk system policy-interrupts ack --dedupe-key ")+latest.DedupeKey).
		Log()
	return errfmt.Errorf("critical policy interrupt requires acknowledgement: %s", latest.DedupeKey)
}

func formatReminderDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.0fm", d.Minutes())
	}
	return fmt.Sprintf("%.1fh", d.Hours())
}

// checkSchedulerDaemonStatus checks if scheduler daemon is running and warns users if not
// Only shows warning to users with appropriate permissions
func checkSchedulerDaemonStatus(projectRoot string, cmd *cobra.Command, _ *cli.Context) error {
	// Skip if no project root
	if projectRoot == EmptyValue {
		return nil
	}
	// Skip when running under test root (e.g. bootstrap CRUD test subprocess) to avoid
	// slow or flaky process/pid checks and to keep test isolation
	if zqkenv.TestRoot().Get() != EmptyValue {
		return nil
	}

	// Check if user has permission to read scheduler status
	// Use authenticated context for permission check
	secCtx := pkgctx.GetSecurityContext(cmd.Context())
	if secCtx == nil {
		secCtx = pkgctx.NewSystemSecurityContext()
	}

	// Check for read:scheduler_job or manage:scheduler permission
	hasPermission := false
	for _, role := range secCtx.Roles {
		if role == "admin" {
			hasPermission = true
			break
		}
	}

	if !hasPermission {
		for _, perm := range secCtx.Permissions {
			if perm == "read:scheduler_job" || perm == "manage:scheduler" || perm == "read:*" || perm == "manage:*" {
				hasPermission = true
				break
			}
		}
	}

	if !hasPermission {
		return nil // Don't show warning if user doesn't have permission
	}

	// Check if scheduler is running (in this process or another).
	// Use GetGlobalSchedulerIfAvailable so we never block on globalSchedulerMu (daemon holds it
	// during job dispatch); blocking here left CLI processes stuck and unkillable in some environments.
	sched, ok := schedulerpkg.GetGlobalSchedulerIfAvailable()
	var isRunning bool
	when.When(func() bool { return ok && sched != nil && sched.IsRunning() }).Then(func() {
		isRunning = true
	}).OrElse(func() {
		running, _, err := schedulerpkg.IsSchedulerRunning(projectRoot)
		isRunning = err == nil && running
	}).Run()

	if !isRunning && isMutatingCommand(cmd) {
		ignore, _ := cmd.Flags().GetBool("allow-degraded")

		if !ignore {
			// POL-CODE-007: user-facing output via logger
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			logging.Fluent(logger).Warn("Scheduler daemon is down. Operations will be queued to WAL.").
				String("impact", "Side-effects and lifecycles will execute when daemon returns").
				String("action", paths.RewriteCanonicalCLIInvocations("Start with: zqk scheduler start")).
				Log()
		}
	}
	return nil
}

// isMutatingCommand returns true when cmd (or any ancestor) is a mutating command
// that depends on the scheduler for side-effects and lifecycle coordination.
// Read-only commands (get, list, count, status, etc.) return false so the
// root-level scheduler guard does not emit noisy false-positive warnings.
func isMutatingCommand(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		switch c.Name() {
		case "create", "update", "delete", "workflow", "system":
			return true
		}
	}
	return false
}

// createCommandAuditEvent creates an audit event for command execution
// Coordinator emits asynchronously, so this function always succeeds
func createCommandAuditEvent(cmd *cobra.Command, projectRoot string, metric *clitool.CommandMetric, profile string) {
	if cmd == nil || projectRoot == EmptyValue {
		return
	}
	operation := buildOperationDescription(metric)
	severity := determineSeverity(metric)
	metadata := buildAuditMetadata(projectRoot, metric)
	//nolint:errcheck // Audit events are fire-and-forget
	_ = validateAndWriteAuditEvent(cmd, projectRoot, operation, severity, metadata, metric, profile)
}
