package orchestration

import (
	"context"
	"fmt"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/rollback"
	"github.com/lanceman/zqk/pkg/storage"
)

// SentinelConfig configures the sentinel checks and retry behavior.
type SentinelConfig struct {
	TruthSentinel           bool
	EnterpriseTruthSentinel bool
	MaxRetries              int
}

// SubagentRunner manages subagent execution with sentinel checks and rollback.
type SubagentRunner struct {
	config      SentinelConfig
	projectRoot string
	provider    storage.ObjectStorageProvider
	capLoop     CAPLoop
}

// CAPAlert represents an alert sent to the Continuous Alignment & Planning loop.
type CAPAlert struct {
	SubagentID string
	Violations []error
	Trace      string
}

// CAPLoop defines the interface for alerting the Continuous Alignment & Planning loop.
type CAPLoop interface {
	Alert(ctx context.Context, alert CAPAlert) error
}

// NewSubagentRunner creates a new SubagentRunner.
func NewSubagentRunner(cfg SentinelConfig, root string, provider storage.ObjectStorageProvider, capLoop CAPLoop) *SubagentRunner {
	return &SubagentRunner{
		config:      cfg,
		projectRoot: root,
		provider:    provider,
		capLoop:     capLoop,
	}
}

// RunSubagentTask executes a task with automatic retries and sentinel rollback.
func (r *SubagentRunner) RunSubagentTask(ctx context.Context, subagentID string, task func(ctx context.Context) error, getStates func() ([]rollback.ObjectState, error)) error {
	logger := logging.NewEventLogger(ctx)

	// Capture initial state before subagent executes
	pointID, err := rollback.Capture(r.projectRoot, "subagent", subagentID, getStates)
	if err != nil {
		return fmt.Errorf("failed to capture rollback point: %w", err)
	}

	var lastViolations []error
	var taskErr error

	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		// Run task
		taskErr = task(ctx)
		if taskErr != nil {
			logger.LogWarning("Subagent task failed", logging.String("subagent", subagentID), logging.Error(taskErr))
			// If task fails we don't automatically run sentinels, we check if we should retry
		}

		// Run Sentinels
		violations := r.runSentinels(ctx, subagentID)

		if taskErr == nil && len(violations) == 0 {
			// Success
			return nil
		}

		lastViolations = violations
		if taskErr != nil {
			lastViolations = append(lastViolations, taskErr)
		}

		// If we still have retries, rollback and try again
		if attempt < r.config.MaxRetries {
			logger.LogInfo("Retrying subagent task", logging.Int("attempt", attempt+1), logging.String("subagent", subagentID))

			// Revert to point before retrying
			if pointID != "" {
				rbErr := rollback.Apply(ctx, r.projectRoot, pointID, r.provider)
				if rbErr != nil {
					return fmt.Errorf("rollback failed during retry: %w", rbErr)
				}
			}
			continue
		}
	}

	// Failure (Fatal/Limit Reached) - Trigger instant rollback & CAP alert
	if pointID != "" {
		rbErr := rollback.Apply(ctx, r.projectRoot, pointID, r.provider)
		if rbErr != nil {
			logger.LogError("Rollback failed after subagent failure", rbErr)
		}
	}

	alert := CAPAlert{
		SubagentID: subagentID,
		Violations: lastViolations,
		Trace:      fmt.Sprintf("Failed after %d retries", r.config.MaxRetries),
	}
	if err := r.capLoop.Alert(ctx, alert); err != nil {
		logger.LogError("Failed to send CAP alert", err)
	}

	return fmt.Errorf("subagent task failed and rolled back after %d retries", r.config.MaxRetries)
}

func (r *SubagentRunner) runSentinels(ctx context.Context, subagentID string) []error {
	var violations []error
	if r.config.TruthSentinel {
		if err := r.runTruthSentinel(ctx, subagentID); err != nil {
			violations = append(violations, err)
		}
	}
	if r.config.EnterpriseTruthSentinel {
		if err := r.runEnterpriseTruthSentinel(ctx, subagentID); err != nil {
			violations = append(violations, err)
		}
	}
	return violations
}

func (r *SubagentRunner) runTruthSentinel(ctx context.Context, subagentID string) error {
	// Mock truth-sentinel validation logic
	return nil
}

func (r *SubagentRunner) runEnterpriseTruthSentinel(ctx context.Context, subagentID string) error {
	// Mock enterprise-truth-sentinel validation logic
	return nil
}
