package automation

import (
	"context"
	"fmt"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
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

	// Embed LoggingContext in context so coordinator logging respects --context profile
	ctx = createContextWithLoggingProfile(ctx, profile)

	coordinator := coordination.NewStorageCoordinator(projectRoot, storageProvider)

	// Build audit metadata from options
	auditMetadata := make(map[string]any)
	if options.Metadata != nil {
		for k, v := range options.Metadata {
			auditMetadata[k] = v
		}
	}
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

// createContextWithLoggingProfile creates a context with LoggingContext embedded from profile string
// This ensures coordinator logging events respect --context profile settings
func createContextWithLoggingProfile(ctx context.Context, profile string) context.Context {
	if ctx == nil {
		ctx = pkgctx.NewSystemContext()
	}

	if profile == emptyValue {
		profile = string(pkgctx.ProfileHuman) // Default
	}

	// Convert profile string to LoggingProfile enum
	var loggingCtx *pkgctx.LoggingContext
	switch profile {
	case string(pkgctx.ProfileMCP):
		loggingCtx = pkgctx.NewLoggingContext(pkgctx.ProfileMCP)
	case string(pkgctx.ProfileSystem):
		loggingCtx = pkgctx.NewSystemLoggingContext()
	case string(pkgctx.ProfileAIAgent):
		loggingCtx = pkgctx.NewLoggingContext(pkgctx.ProfileAIAgent)
	case string(pkgctx.ProfileDebug):
		loggingCtx = pkgctx.NewLoggingContext(pkgctx.ProfileDebug)
	case string(pkgctx.ProfileHuman), "":
		loggingCtx = pkgctx.NewHumanLoggingContext()
	default:
		loggingCtx = pkgctx.NewHumanLoggingContext()
	}

	return pkgctx.WithLoggingContext(ctx, loggingCtx)
}

// minInt returns the minimum of two integers
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
