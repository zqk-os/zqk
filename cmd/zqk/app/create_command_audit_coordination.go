package app

import (
	"context"
	"fmt"

	clitool "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
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

// createContextWithLoggingProfile creates a context with LoggingContext embedded from profile string
// This ensures coordinator logging events respect --context profile settings
func createContextWithLoggingProfile(ctx context.Context, profile string) context.Context {
	if ctx == nil {
		ctx = pkgctx.NewSystemContext()
	}

	if profile == EmptyValue {
		profile = profileHuman // Default
	}

	// Convert profile string to LoggingProfile enum
	var loggingCtx *pkgctx.LoggingContext
	switch profile {
	case profileMCP:
		loggingCtx = pkgctx.NewLoggingContext(pkgctx.ProfileMCP)
	case profileSystem:
		loggingCtx = pkgctx.NewSystemLoggingContext()
	case profileAIAgent:
		loggingCtx = pkgctx.NewLoggingContext(pkgctx.ProfileAIAgent)
	case profileDebug:
		loggingCtx = pkgctx.NewLoggingContext(pkgctx.ProfileDebug)
	case profileHuman, "":
		loggingCtx = pkgctx.NewHumanLoggingContext()
	default:
		loggingCtx = pkgctx.NewHumanLoggingContext()
	}

	return pkgctx.WithLoggingContext(ctx, loggingCtx)
}

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

	// Embed LoggingContext in context so coordinator logging respects --context profile
	ctx = createContextWithLoggingProfile(ctx, profile)

	coordinator := coordination.NewStorageCoordinator(projectRoot, storageProvider)

	// Build audit metadata
	auditMetadata := make(map[string]any)
	for k, v := range metadata {
		auditMetadata[k] = v
	}
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
