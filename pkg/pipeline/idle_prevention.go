package pipeline

import (
	"context"
	"fmt"
	"time"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

// IdleDetector defines an interface for detecting if an agent process is idle.
type IdleDetector interface {
	IsIdle(ctx context.Context, agentID string) (bool, error)
	MarkActive(ctx context.Context, agentID string) error
}

// ChatResponder defines an interface for engaging an idle agent.
type ChatResponder interface {
	Engage(ctx context.Context, agentID string, message string) error
}

// IdlePreventionOptions configures the IdlePrevention mechanism.
type IdlePreventionOptions struct {
	Detector      IdleDetector
	Responder     ChatResponder
	IdleThreshold time.Duration
	WakeupMessage string
}

// IdlePreventionMiddleware wraps a StageFunc (like AgentStage) to monitor and prevent agent idle stalling.
// It checks periodically if the agent is idle and engages it using the ChatResponder.
func IdlePreventionMiddleware(agentID string, opts IdlePreventionOptions, next StageFunc) StageFunc {
	return func(pctx *Context, payload any) (any, error) {
		ctx, cancel := context.WithCancel(pctx.Ctx)
		defer cancel()

		if opts.Detector == nil || opts.Responder == nil {
			return nil, fmt.Errorf("IdlePreventionMiddleware requires valid Detector and Responder")
		}

		if opts.IdleThreshold <= 0 {
			opts.IdleThreshold = 5 * time.Minute
		}
		if opts.WakeupMessage == "" {
			opts.WakeupMessage = "Agent is idle. Please provide a status update or continue execution."
		}

		// Run idle monitor in background
		goroutinelabels.StartNamedGoroutine("idle-prevention", "prevent idle timeout", func() {
			func(ctx context.Context) {
				ticker := time.NewTicker(opts.IdleThreshold / 2)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						idle, err := opts.Detector.IsIdle(ctx, agentID)
						if err == nil && idle {
							// Attempt to wake up the agent
							_ = opts.Responder.Engage(ctx, agentID, opts.WakeupMessage)
							// Optimistically mark as active to prevent spam
							_ = opts.Detector.MarkActive(ctx, agentID)
						}
					}
				}
			}(ctx)
		})

		// Execute the wrapped stage
		return next(pctx, payload)
	}
}
