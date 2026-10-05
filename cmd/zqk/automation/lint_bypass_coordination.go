package automation

import (
	"context"
	"fmt"
	"strings"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/storage"
)

// emitLintBypassAuditEventViaCoordinator emits lint bypass audit events via the coordination system
// This replaces direct CreateLintBypassAuditEvent calls
func emitLintBypassAuditEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	gitUser, gitEmail, commitMessage string,
	stagedFiles []string,
	profile string, // CLI context profile for logging format
) {
	if projectRoot == emptyValue || projectRoot == "." {
		// Best effort - skip if no project root
		return
	}

	// Use git user info as actor
	actor := gitUser
	if actor == emptyValue {
		actor = "unknown"
	}
	if gitEmail != emptyValue {
		actor = fmt.Sprintf("%s <%s>", actor, gitEmail)
	}

	// Build operation description
	operation := "Lint checks bypassed with --no-verify flag"
	if commitMessage != emptyValue {
		// Truncate commit message if too long
		msgPreview := commitMessage
		if len(msgPreview) > 100 {
			msgPreview = msgPreview[:100] + "..."
		}
		operation = fmt.Sprintf("Lint checks bypassed: %s", msgPreview)
	}

	// Build metadata
	metadata := map[string]any{
		automationAuditKeySource:      automationSourceGitHook,
		automationAuditKeyProjectRoot: projectRoot,
		"git_user":                    gitUser,
		"git_email":                   gitEmail,
		"commit_message":              commitMessage,
		"staged_files":                stagedFiles,
		"file_count":                  len(stagedFiles),
	}

	// Build audit event options
	options := &storage.AuditEventOptions{
		EventType:  automationAuditCodeQualityBypass,
		Operation:  operation,
		TargetKind: automationTargetKindLint,
		Severity:   automationSeverityMedium, // Medium severity - code quality concern
		Metadata:   metadata,
		CreatedBy:  actor, // Git user info as actor
	}

	ctx, coordinator, auditMetadata := cli.InitAuditCoordination(ctx, projectRoot, storageProvider, profile, options.Metadata)
	auditMetadata[automationAuditKeyEventType] = options.EventType
	auditMetadata[automationAuditKeyOperation] = options.Operation
	auditMetadata[automationAuditKeySeverity] = options.Severity
	auditMetadata[automationAuditKeyTargetKind] = options.TargetKind
	if options.TargetID != emptyValue {
		auditMetadata[automationAuditKeyTargetID] = options.TargetID
	}
	if options.TargetPath != emptyValue {
		auditMetadata[automationAuditKeyTargetPath] = options.TargetPath
	}

	// Create event data
	eventData := &coordination.EventData{
		LoggingFields: nil, // Lint bypass events don't need logging fields
		AuditMetadata: auditMetadata,
		MetricsData:   nil, // Lint bypass events don't create metrics
	}

	// Determine status based on event type
	status := automationStatusComplete

	// Create operation ID from event type
	operationID := fmt.Sprintf("lint_bypass_%s", strings.Join(stagedFiles[:minInt(3, len(stagedFiles))], "_"))
	if len(operationID) > 50 {
		operationID = operationID[:50]
	}

	// Create event context (only audit channel enabled)
	eventCtx := coordination.NewEventContext(operationID, automationEventTypeLintBypass, status).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(false, true, false, false) // Only audit, no logging/metrics/operational

	// Emit via coordinator (async, non-blocking)
	goroutinelabels.NewGoroutine("lint_bypass_event_emitter", "emitting lint bypass event").
		StartSimple(func() {
			_ = coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
		})
}

// minInt returns the minimum of two integers
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
