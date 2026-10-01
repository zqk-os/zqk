package coordination

import (
	"context"
	"fmt"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/metrics"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// AuditEventCallback is a callback function called when an audit event is created (success or failure)
// Used for test verification and monitoring
type AuditEventCallback func(eventType string, operationID string, err error)

// StorageAuditRouter routes events to the audit channel using storage.CreateAuditEventWithBuilder
type StorageAuditRouter struct {
	projectRoot string
	storage     storage.ObjectStorageProvider
	secCtx      *pkgctx.SecurityContext
	callback    AuditEventCallback // Optional callback for test verification
}

// NewStorageAuditRouter creates a new audit router that uses storage.CreateAuditEventWithBuilder
func NewStorageAuditRouter(projectRoot string, storageProvider storage.ObjectStorageProvider) *StorageAuditRouter {
	return &StorageAuditRouter{
		projectRoot: projectRoot,
		storage:     storageProvider,
		secCtx:      pkgctx.NewSystemSecurityContext(),
		callback:    nil,
	}
}

// DefaultAuditRouter creates a new audit router that uses storage.CreateAuditEventWithBuilder.
func DefaultAuditRouter(projectRoot string, storageProvider storage.ObjectStorageProvider) AuditRouter {
	return NewStorageAuditRouter(projectRoot, storageProvider)
}

// SetCallback sets an optional callback function to be called when audit events are created
// This is useful for test verification and monitoring async operations
func (r *StorageAuditRouter) SetCallback(callback AuditEventCallback) {
	r.callback = callback
}

// HasCallback returns true if a callback is set (used by coordinator to run audit sync in tests)
func (r *StorageAuditRouter) HasCallback() bool {
	return r.callback != nil
}

// Emit creates an audit event from the EventContext
func (r *StorageAuditRouter) Emit(ctx context.Context, eventCtx *EventContext) error {
	if r.projectRoot == emptyValue || eventCtx.EventData == nil || eventCtx.EventData.AuditMetadata == nil {
		return nil // Best effort - skip if no project root or metadata
	}

	// Extract event type and operation from metadata
	eventType, _ := eventCtx.EventData.AuditMetadata[objects.FieldKeyEventType].(string)
	if eventType == emptyValue {
		// Try to infer from operation type and status
		eventType = r.inferEventType(eventCtx.OperationType, eventCtx.Status)
	}

	operation, _ := eventCtx.EventData.AuditMetadata[objects.FieldKeyOperation].(string)
	if operation == emptyValue {
		operation = fmt.Sprintf("%s: %s", eventCtx.OperationType, eventCtx.Status)
	}

	severity, _ := eventCtx.EventData.AuditMetadata[objects.FieldKeySeverity].(string)
	if severity == emptyValue {
		severity = r.inferSeverity(eventCtx.Status)
	}

	// Extract other metadata fields
	targetKind, _ := eventCtx.EventData.AuditMetadata[objects.FieldKeyTargetKind].(string)
	targetID, _ := eventCtx.EventData.AuditMetadata[objects.FieldKeyTargetID].(string)
	targetPath, _ := eventCtx.EventData.AuditMetadata[objects.FieldKeyTargetPath].(string)
	sessionID, _ := eventCtx.EventData.AuditMetadata[objects.FieldKeySessionID].(string)

	// Normalize event type against allowed list to avoid validation failures
	normalizedType, normalizedMeta := storage.NormalizeAuditEventType(eventType, eventCtx.EventData.AuditMetadata)

	// Create audit event options with error callback for test verification
	var createErr error
	options := &storage.AuditEventOptions{
		EventType:  normalizedType,
		Operation:  operation,
		Severity:   severity,
		TargetKind: targetKind,
		TargetID:   targetID,
		TargetPath: targetPath,
		SessionID:  sessionID,
		Metadata:   normalizedMeta,
		CreatedBy:  r.secCtx.AccountID,
		OnError: func(err error) {
			// Capture error for callback
			createErr = err
		},
	}

	// Create audit event
	err := storage.CreateAuditEventWithBuilder(ctx, r.projectRoot, r.secCtx, r.storage, options)
	if err != nil && createErr == nil {
		createErr = err
	}

	// Call callback if set (for test verification)
	// Pass the actual error captured from OnError callback
	if r.callback != nil {
		r.callback(eventType, eventCtx.OperationID, createErr)
	}

	if err != nil {
		return err
	}
	return nil
}

// inferEventType infers event type from operation type and status
// Maps operation types to valid audit event types from the audit_event spec
func (r *StorageAuditRouter) inferEventType(operationType, status string) string {
	// Special handling for orphan_cleanup operations to ensure valid event types
	// Valid types: orphan_cleanup_start, orphan_cleanup_complete, orphan_cleanup_error
	if strings.HasPrefix(operationType, "orphan_cleanup") {
		if status == OperationStatusError || strings.Contains(status, "failure") {
			return "orphan_cleanup_error"
		}
		if status == OperationStatusStart || operationType == "orphan_cleanup_worker_started" {
			return "orphan_cleanup_start"
		}
		// Default to complete for batch operations and other statuses
		return "orphan_cleanup_complete"
	}

	// Standard inference for other operation types
	if status == OperationStatusError {
		return fmt.Sprintf("%s_error", operationType)
	}
	if status == OperationStatusComplete {
		return fmt.Sprintf("%s_complete", operationType)
	}
	if status == OperationStatusStart {
		return fmt.Sprintf("%s_start", operationType)
	}
	return fmt.Sprintf("%s_%s", operationType, status)
}

// inferSeverity infers severity from status
func (r *StorageAuditRouter) inferSeverity(status string) string {
	switch status {
	case OperationStatusError:
		return "high"
	case OperationStatusComplete:
		return "medium"
	default:
		return "low"
	}
}

// MetricPipelineRouter routes events to the metrics channel using MetricPipeline.Sample
type MetricPipelineRouter struct {
	pipeline *metrics.MetricPipeline
}

// NewMetricPipelineRouter creates a new metrics router that uses MetricPipeline.Sample
func NewMetricPipelineRouter(pipeline *metrics.MetricPipeline) *MetricPipelineRouter {
	return &MetricPipelineRouter{
		pipeline: pipeline,
	}
}

// Emit sends metrics data to the MetricPipeline
func (r *MetricPipelineRouter) Emit(ctx context.Context, eventCtx *EventContext) error {
	if r.pipeline == nil || eventCtx.EventData == nil || eventCtx.EventData.MetricsData == nil {
		return nil // Best effort - skip if no pipeline or metrics data
	}

	// MetricPipeline.Sample expects a map[string]any
	// Use the metrics data directly
	_, err := r.pipeline.Sample(eventCtx.EventData.MetricsData)
	return err // Return error but don't fail operation (best effort)
}
