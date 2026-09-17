package system

import (
	"context"
	"fmt"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/coordination"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
)

// emitCacheRefreshAuditEventViaCoordinator emits cache refresh audit events via the coordination system
// This replaces direct CreateCacheRefreshAuditEvent calls
func emitCacheRefreshAuditEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	commandArgs []string,
	systemState map[string]any,
	profile string, // CLI context profile for logging format
) {
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	if projectRoot == emptyValue {
		// Best effort - skip if no project root
		return
	}

	// Build operation description
	operation := "Cache refresh requested - all caches cleared for revalidation against latest specs"
	if len(commandArgs) > 0 {
		operation = fmt.Sprintf("Cache refresh: %s", strings.Join(commandArgs, " "))
	}

	// Build comprehensive metadata with system state
	metadata := map[string]any{
		eventKeySource:              "cli",
		eventKeyProjectRoot:         projectRoot,
		objects.FieldKeyCommand:     fmt.Sprintf("%s system check --refresh-cache", paths.CLICommandName),
		"args":                      commandArgs,
		objects.FieldKeyRoles:       secCtx.Roles,
		objects.FieldKeyPermissions: secCtx.Permissions,
	}

	// Add system state information
	mergeMetadata(metadata, systemState)

	// Build audit event options
	options := &storage.AuditEventOptions{
		EventType:  eventTypeSystemConfigChange, // Cache refresh is a system configuration change
		Operation:  operation,
		TargetKind: targetKindCache, // Target is the cache system
		Severity:   severityHigh,    // High severity - affects validation rules
		Metadata:   metadata,
		CreatedBy:  secCtx.AccountID,
	}

	// Call the existing coordinator helper
	emitCacheAuditEventViaCoordinator(ctx, projectRoot, storageProvider, secCtx, options, profile)
}

// emitCacheAuditEventViaCoordinator emits cache audit events via the coordination system
// This replaces direct CreateAuditEventWithBuilder calls for cache audit events
func emitCacheAuditEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	secCtx *pkgctx.SecurityContext,
	options *storage.AuditEventOptions,
	profile string, // CLI context profile for logging format
) {
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	if projectRoot == emptyValue {
		// Best effort - skip if no project root
		return
	}

	// Embed LoggingContext in context so coordinator logging respects --context profile
	ctx = createContextWithLoggingProfile(ctx, profile)

	// Create routers for coordinator
	auditRouter := coordination.NewStorageAuditRouter(projectRoot, storageProvider)

	// Create coordinator with routers (only audit for cache events)
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		AuditRouter:       auditRouter,
		MetricsRouter:     nil, // Cache audit events don't need metrics router
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	// Build audit metadata from options
	auditMetadata := make(map[string]any)
	mergeMetadata(auditMetadata, options.Metadata)
	auditMetadata[eventKeyEventType] = options.EventType
	auditMetadata[eventKeyOperation] = options.Operation
	auditMetadata[eventKeySeverity] = options.Severity
	auditMetadata[eventKeyTargetKind] = options.TargetKind
	if options.TargetID != emptyValue {
		auditMetadata[eventKeyTargetID] = options.TargetID
	}
	if options.TargetPath != emptyValue {
		auditMetadata[objects.FieldKeyTargetPath] = options.TargetPath
	}

	// Create event data
	eventData := &coordination.EventData{
		LoggingFields: nil, // Cache events don't need logging fields
		AuditMetadata: auditMetadata,
		MetricsData:   nil, // Cache events don't create metrics
	}

	// Determine status based on event type
	status := eventStatusComplete
	if options.Severity == severityHigh {
		status = eventStatusError
	}

	// Create operation ID from event type and target
	operationID := fmt.Sprintf("cache_%s", options.EventType)
	if options.TargetID != emptyValue {
		operationID = fmt.Sprintf("cache_%s_%s", options.EventType, options.TargetID)
	}

	// Create event context (only audit channel enabled)
	eventCtx := coordination.NewEventContext(operationID, eventTypeCacheOperation, status).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(false, true, false, false) // Only audit, no logging/metrics/operational

	// Emit via coordinator (async, non-blocking)
	goroutinelabels.NewGoroutine("check_cache_audit_emitter", "emitting check cache audit event").
		StartSimple(func() {
			_ = coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
		})
}
