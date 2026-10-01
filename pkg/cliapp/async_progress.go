package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	pkgcli "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/diagnostics"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

const emptyValue = ""

// progressHeartbeatInterval is the maximum time the user can go without a progress message.
// Ensures we never leave the user wondering; matches pattern-cli and system check (5–7s).
const progressHeartbeatInterval = 5 * time.Second

// progressHeartbeatMaxDuration caps how long the heartbeat runs. Long-running commands (e.g. daemon
// "scheduler start" in foreground) would otherwise emit every 5s forever and flood diagnostics.jsonl.
const progressHeartbeatMaxDuration = 2 * time.Minute

const (
	progressStatusStarted    = "started"
	progressStatusInProgress = "in_progress"
)

// OperationTypeFromCommand returns a stable operation type string from cmd's full path,
// e.g. "zqk spec list" → "spec_list", "zqk object delete" → "object_delete".
// Use for coordinator events and metrics. Safe to call from RunE (cmd is in the tree).
func OperationTypeFromCommand(cmd *cobra.Command) string {
	path := cmd.CommandPath()
	parts := strings.Fields(path)
	if len(parts) < 2 {
		return "cli"
	}
	return strings.Join(parts[1:], "_")
}

// BindAsyncProgress sets cmd.RunE to run runE wrapped with RunWithAsyncProgress.
// Operation type is derived at runtime from cmd.CommandPath() so it stays correct
// for subcommands (e.g. "zqk spec list" → "spec_list"). Use for consistent async retrofit.
func BindAsyncProgress(cmd *cobra.Command, runE func(*cobra.Command, []string) error) {
	cmd.RunE = func(c *cobra.Command, args []string) error {
		return RunWithAsyncProgress(c, args, OperationTypeFromCommand(c), runE)
	}
}

// NewAsyncCommand creates a cobra.Command with Use, Short, Long, RunE wrapped with
// BindAsyncProgress, and common flags. Use for hand-built commands that follow the
// async pattern and need no custom flags or help builder.
func NewAsyncCommand(use, short, long string, runE func(*cobra.Command, []string) error) *cobra.Command {
	b := pkgcli.NewCommandBuilder(use).
		WithShort(short).
		WithLong(long).
		WithRunE(func(c *cobra.Command, args []string) error {
			return RunWithAsyncProgress(c, args, OperationTypeFromCommand(c), runE)
		})

	// Add common flags (assuming cli.AddCommonFlags is available in the package)
	b.WithCommonFlagsDefault(AddCommonFlags)

	return b.Build()
}

// progressState holds the latest stage/message for heartbeat emission. Safe for concurrent use.
type progressState struct {
	mu      sync.Mutex
	stage   string
	message string
}

func (p *progressState) set(stage, message string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stage = stage
	p.message = message
}

func (p *progressState) get() (stage, message string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stage, p.message
}

// progressHeartbeatLabel returns a user-facing label for heartbeat messages so daemon/long-running
// commands don't imply "still starting" (e.g. "Scheduler daemon running" not "Operation in progress: scheduler_start").
func progressHeartbeatLabel(operationType string) string {
	switch operationType {
	case "scheduler_start":
		return "Scheduler daemon running"
	case "mcp_serve":
		return "MCP server running"
	default:
		return fmt.Sprintf("Operation in progress: %s", operationType)
	}
}

// runProgressHeartbeat emits a status message every interval so the user never goes too long without feedback.
// Stops when ctx is cancelled (e.g. when runE returns) or after progressHeartbeatMaxDuration so daemon
// processes (e.g. scheduler start --foreground) do not flood the event log indefinitely.
func runProgressHeartbeat(
	ctx context.Context,
	helper *coordination.ProgressHelper,
	operationType string,
	state *progressState,
	interval time.Duration,
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	deadline := time.Now().Add(progressHeartbeatMaxDuration)
	baseLabel := progressHeartbeatLabel(operationType)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if time.Now().After(deadline) {
				return
			}
			stage, message := state.get()
			msg := baseLabel
			if stage != emptyValue || message != emptyValue {
				if stage != emptyValue && message != emptyValue {
					msg = fmt.Sprintf("%s — %s: %s", baseLabel, stage, message)
				} else if message != emptyValue {
					msg = fmt.Sprintf("%s — %s", baseLabel, message)
				} else {
					msg = fmt.Sprintf("%s — %s", baseLabel, stage)
				}
			}
			// Use in_progress -> in_progress for heartbeats so they are not deduped (user sees routine progress)
			_ = helper.EmitStatusChange(ctx, progressStatusInProgress, progressStatusInProgress, msg, nil)
		}
	}
}

// RunWithAsyncProgress wraps a RunE with the async CLI pattern: ack (started) → run → complete or error.
// It attaches a validation progress callback to cmd.Context() so storage/validator can emit
// stage progress (e.g. "spec", "lifecycle"). A heartbeat goroutine emits status on a regular interval
// so the user is never left without feedback (no silent hangs).
func RunWithAsyncProgress(
	cmd *cobra.Command,
	args []string,
	operationType string,
	runE func(cmd *cobra.Command, args []string) error,
) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	projectRoot := ResolveProjectRoot(".")
	profile := GetProfile(cmd)
	if profile == emptyValue {
		profile = string(pkgctx.ProfileHuman)
	}
	operationID := fmt.Sprintf("%s_%d", operationType, time.Now().UnixNano())
	ec := coordination.GetCoordinator()
	var coord *coordination.Coordinator
	if c, ok := ec.(*coordination.Coordinator); ok {
		coord = c
	}
	helper := coordination.NewProgressHelper(coord, projectRoot, operationID, operationType, profile)

	// Shared state so heartbeat can report latest stage/message
	var state progressState
	progressFn := func(stage, message string) {
		state.set(stage, message)
		_ = helper.EmitStatusChange(ctx, progressStatusStarted, stage, message, nil)
	}
	cmd.SetContext(pkgctx.WithValidationProgress(ctx, progressFn))

	// Heartbeat: emit status every N seconds so user never wonders what's going on
	heartbeatCtx, heartbeatCancel := context.WithCancel(ctx)
	defer heartbeatCancel()
	goroutinelabels.NewGoroutine("ProgressHeartbeat", "emit status every N seconds").StartSimple(func() { runProgressHeartbeat(heartbeatCtx, helper, operationType, &state, progressHeartbeatInterval) })

	// Setup SIGUSR1 handler for diagnostics capture (useful for long-running commands)
	// Diagnostics are written to .zqk/diagnostics/<operation_type>/
	diagnostics.SetupSignalHandler(ctx, projectRoot, operationType)

	start := time.Now()
	err := runE(cmd, args)
	duration := time.Since(start)

	heartbeatCancel() // stop heartbeat as soon as runE returns

	if err != nil {
		var exitCoder interface{ ExitCode() int }
		if errors.As(err, &exitCoder) && exitCoder.ExitCode() == 3 {
			_ = helper.EmitCompletion(ctx, duration, "Complete with warnings", nil)
			return err
		}
		_ = helper.EmitError(ctx, err, "Operation failed", nil)
		return err
	}
	_ = helper.EmitCompletion(ctx, duration, "Complete", nil)
	return nil
}
