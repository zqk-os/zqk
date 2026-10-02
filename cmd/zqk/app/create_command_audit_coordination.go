package app

import (
	"context"
	"fmt"

	clitool "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/storage"
)

const (
	profileHuman   = "human"
	profileMCP     = "mcp"
	profileSystem  = "system"
	profileAIAgent = "ai-agent"
	profileDebug   = "debug"
)

const (
	commandExecutionEventType = "command_execution"
	commandExecutionStatusOK  = "complete"
	commandExecutionStatusErr = "error"
	commandExecutionTarget    = "system"
	commandEventKeyType       = "event_type"
	commandEventKeyOperation  = "operation"
	commandEventKeySeverity   = "severity"
	commandEventKeyTargetKind = "target_kind"
)

// emitCommandExecutionEventViaCoordinator emits command execution events via the coordination system
// This replaces direct CreateAuditEventWithBuilder calls for command execution audit events
func emitCommandExecutionEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operation string,
	severity string,
	metadata map[string]any,
	metric *clitool.CommandMetric,
	profile string, // CLI context profile for logging format
) {
	if projectRoot == EmptyValue || projectRoot == "." {
		// Best effort - skip if no project root
		return
	}

	ctx, coordinator, auditMetadata := cli.InitAuditCoordination(ctx, projectRoot, storageProvider, profile, metadata)
	auditMetadata[commandEventKeyType] = commandExecutionEventType
	auditMetadata[commandEventKeyOperation] = operation
	auditMetadata[commandEventKeySeverity] = severity
	auditMetadata[commandEventKeyTargetKind] = commandExecutionTarget

	// Build logging fields (optional - for structured logging)
	loggingFields := []coordination.LoggingField{
		{Key: commandEventKeyOperation, Value: operation},
		{Key: "command", Value: metric.Command},
		{Key: commandEventKeySeverity, Value: severity},
		{Key: "success", Value: metric.Success},
		{Key: "duration_ms", Value: metric.Duration.Milliseconds()},
	}
	if metric.Error != EmptyValue {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: "error", Value: metric.Error})
	}

	// Create event data
	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: auditMetadata,
		MetricsData:   nil, // Command execution events don't create metrics
	}

	// Determine status based on success
	status := commandExecutionStatusOK
	if !metric.Success {
		status = commandExecutionStatusErr
	}

	// Create operation ID from command and timestamp
	operationID := fmt.Sprintf("cmd_%s_%d", metric.NormalizedCmd, metric.StartTime.Unix())

	// Create event context (only audit channel enabled)
	eventCtx := coordination.NewEventContext(operationID, commandExecutionEventType, status).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(false, true, false, false) // Only audit, no logging/metrics/operational

	if !metric.Success && metric.Error != EmptyValue {
		eventCtx = eventCtx.WithError(errfmt.Errorf("%s", metric.Error))
	}

	if metric.Duration > 0 {
		eventCtx = eventCtx.WithDuration(metric.Duration)
	}

	// Emit via coordinator (async, non-blocking)
	goroutinelabels.NewGoroutine("create_command_audit_emitter", "emitting create command audit event").
		StartSimple(func() {
			_ = coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
		})
}
