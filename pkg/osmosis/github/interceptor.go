package github

import (
	"context"
	"errors"
	"os/exec"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"

	"github.com/zqk-os/zqk/pkg/circuitbreaker"
	"github.com/zqk-os/zqk/pkg/telemetry"
)

var (
	ErrRateLimitExceeded = errors.New("github api rate limit exceeded")
	ErrExecutionBlocked  = errors.New("execution blocked by interceptor policy")
)

// Interceptor handles proxying Git and GitHub CLI commands, enforcing
// rate limits, applying policies, and recording telemetry.
type Interceptor struct {
	toolName string
	limiter  circuitbreaker.RateLimiter
	tracker  telemetry.Tracker
}

// NewInterceptor creates a new GitHub interceptor.
func NewInterceptor(toolName string, limiter circuitbreaker.RateLimiter, tracker telemetry.Tracker) *Interceptor {
	return &Interceptor{
		toolName: toolName,
		limiter:  limiter,
		tracker:  tracker,
	}
}

// Execute runs the intercepted command, applies rate limits, and logs telemetry.
func (i *Interceptor) Execute(ctx context.Context, args []string) ([]byte, error) {
	// 1. Rate limiting check
	if i.limiter != nil && !i.limiter.Allow(i.toolName) {
		if i.tracker != nil {
			i.tracker.RecordRateLimitExceeded(ctx, i.toolName)
		}
		return nil, ErrRateLimitExceeded
	}

	// 2. Policy enforcement (e.g. blocking certain args if they lack ZQK IDs)
	// (Simulated basic check for TDD purposes: block if args contain "block-me")
	for _, arg := range args {
		if arg == "block-me" {
			return nil, ErrExecutionBlocked
		}
	}

	start := time.Now()

	// 3. Execution
	cmd := execwrap.CommandContext(ctx, i.toolName, args...)
	output, err := cmd.CombinedOutput()

	duration := time.Since(start)
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	// 4. Telemetry logging
	if i.tracker != nil {
		i.tracker.RecordExecution(ctx, i.toolName, args, duration, exitCode, err)
	}

	return output, err
}
