package coordination

import (
	"context"
	"time"
)

// CLINotifierAdapter adapts ProgressHelper to the storage.ProgressEventEmitter interface
// This allows CLINotifier to use coordinator without creating import cycles
type CLINotifierAdapter struct {
	helper *ProgressHelper
}

// NewCLINotifierAdapter creates a new adapter for CLINotifier
func NewCLINotifierAdapter(helper *ProgressHelper) *CLINotifierAdapter {
	return &CLINotifierAdapter{
		helper: helper,
	}
}

// EmitProgress emits a progress update event
func (a *CLINotifierAdapter) EmitProgress(
	ctx context.Context,
	operationID, operationType string,
	progress, total int,
	message string,
	fields map[string]any,
	emitAudit bool,
) error {
	if a.helper == nil {
		return nil
	}
	return a.helper.EmitProgress(ctx, progress, total, message, fields, emitAudit)
}

// EmitStatusChange emits a status change event
func (a *CLINotifierAdapter) EmitStatusChange(
	ctx context.Context,
	operationID, operationType, oldStatus, newStatus, message string,
	fields map[string]any,
) error {
	if a.helper == nil {
		return nil
	}
	return a.helper.EmitStatusChange(ctx, oldStatus, newStatus, message, fields)
}

// EmitError emits an error event
func (a *CLINotifierAdapter) EmitError(
	ctx context.Context,
	operationID, operationType string,
	err error,
	message string,
	fields map[string]any,
) error {
	if a.helper == nil {
		return nil
	}
	return a.helper.EmitError(ctx, err, message, fields)
}

// EmitCompletion emits a completion event
func (a *CLINotifierAdapter) EmitCompletion(
	ctx context.Context,
	operationID, operationType string,
	duration time.Duration,
	message string,
	fields map[string]any,
) error {
	if a.helper == nil {
		return nil
	}
	return a.helper.EmitCompletion(ctx, duration, message, fields)
}
