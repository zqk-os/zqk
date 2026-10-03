package dispatch

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/diagnostics"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// ProgressHeartbeatInterval is the maximum time the user can go without a progress message.
// See docs/architecture/CLI_ASYNC_AND_PROGRESS.md.
const ProgressHeartbeatInterval = 5 * time.Second

const (
	statusStarted              = "started"
	statusInProgress           = "in_progress"
	emptyProgressValue         = ""
	messageOpPrefix            = "Operation in progress: %s"
	messageOpStage             = "Operation in progress: %s — %s: %s"
	messageOpDetail            = "Operation in progress: %s — %s"
	completeMessage            = "Complete"
	errorMessage               = "Operation failed"
	goroutineProgressHeartbeat = "dispatch_progress_heartbeat"
	goroutineHeartbeatFmt      = "heartbeat for %s"
)

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

func runProgressHeartbeat(
	ctx context.Context,
	helper *coordination.ProgressHelper,
	operationType string,
	state *progressState,
	interval time.Duration,
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stage, message := state.get()
			msg := fmt.Sprintf(messageOpPrefix, operationType)
			if stage != emptyProgressValue || message != emptyProgressValue {
				if stage != emptyProgressValue && message != emptyProgressValue {
					msg = fmt.Sprintf(messageOpStage, operationType, stage, message)
				} else if message != emptyProgressValue {
					msg = fmt.Sprintf(messageOpDetail, operationType, message)
				} else {
					msg = fmt.Sprintf(messageOpDetail, operationType, stage)
				}
			}
			_ = helper.EmitStatusChange(ctx, statusStarted, statusInProgress, msg, nil)
		}
	}
}

// runWithProgress runs runner with progress callback on context and heartbeat to the Coordinator.
// Kept in pkg/dispatch to avoid import cycle (pkg/cli -> coordination -> storage -> pkg/cli).
func runWithProgress(
	ctx context.Context,
	item *WorkItem,
	runner func(ctx context.Context) error,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if item.Profile == emptyProgressValue {
		item.Profile = string(pkgctx.ProfileHuman)
	}
	helper := coordination.NewCoordinatorProgressHelper(
		item.ProjectRoot, item.OperationID, item.OperationType, item.Profile,
	)

	var state progressState
	progressFn := func(stage, message string) {
		state.set(stage, message)
		_ = helper.EmitStatusChange(ctx, statusStarted, stage, message, nil)
	}
	ctxWithProgress := pkgctx.WithValidationProgress(ctx, progressFn)

	heartbeatCtx, heartbeatCancel := context.WithCancel(ctx)
	defer heartbeatCancel()
	goroutinelabels.NewGoroutine(goroutineProgressHeartbeat, fmt.Sprintf(goroutineHeartbeatFmt, item.OperationType)).
		StartSimple(func() {
			runProgressHeartbeat(heartbeatCtx, helper, item.OperationType, &state, ProgressHeartbeatInterval)
		})

	diagnostics.SetupSignalHandler(ctxWithProgress, item.ProjectRoot, item.OperationType)

	start := time.Now()
	err := runner(ctxWithProgress)
	duration := time.Since(start)

	heartbeatCancel()

	if err != nil {
		var exitCoder interface{ ExitCode() int }
		if errors.As(err, &exitCoder) && exitCoder.ExitCode() == 3 {
			_ = helper.EmitCompletion(ctxWithProgress, duration, "Complete with warnings", nil)
			return err
		}
		_ = helper.EmitError(ctxWithProgress, err, errorMessage, nil)
		return err
	}
	_ = helper.EmitCompletion(ctxWithProgress, duration, completeMessage, nil)
	return nil
}
