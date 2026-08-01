package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/lanceman/zqk/internal/cli"
	clitool "github.com/lanceman/zqk/pkg/cli"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/spf13/cobra"
)

const (
	auditSeverityHigh   = "high"
	auditSeverityMedium = "medium"
	auditSeverityLow    = "low"
)

const (
	auditMetaSource         = "source"
	auditMetaProjectRoot    = "project_root"
	auditMetaCommand        = "command"
	auditMetaError          = "error"
	auditMetaNormalizedCmd  = "normalized_cmd"
	auditMetaArgs           = "args"
	auditMetaFlags          = "flags"
	auditMetaDurationMs     = "duration_ms"
	auditMetaStartTime      = "start_time"
	auditMetaEndTime        = "end_time"
	auditMetaExitCode       = "exit_code"
	auditMetaTimedOut       = "timed_out"
	auditMetaObjectsCreated = "objects_created"
	auditMetaObjectsUpdated = "objects_updated"
	auditMetaObjectsDeleted = "objects_deleted"
	auditMetaPriorityPlan   = objects.KindPriorityPlan
	auditMetaWorkstream     = objects.KindWorkstream
	auditMetaMilestone      = objects.KindMilestone
	auditMetaActorID        = "actor_id"
	auditMetaActorRoles     = "actor_roles"
)

// buildOperationDescription builds the operation description from command and args
func buildOperationDescription(metric *clitool.CommandMetric) string {
	operation := fmt.Sprintf("Command execution: %s", metric.Command)
	if len(metric.Args) > 0 {
		operation += " " + strings.Join(metric.Args, " ")
	}
	return operation
}

// determineSeverity determines severity based on outcome and state changes
func determineSeverity(metric *clitool.CommandMetric) string {
	if !metric.Success {
		return auditSeverityHigh
	}
	if len(metric.ObjectsDeleted) > 0 {
		return auditSeverityHigh
	}
	if len(metric.ObjectsCreated) > 0 || len(metric.ObjectsUpdated) > 0 {
		return auditSeverityMedium
	}
	return auditSeverityLow
}

// buildAuditMetadata builds comprehensive metadata for the audit event
func buildAuditMetadata(projectRoot string, metric *clitool.CommandMetric) map[string]any {
	metadata := map[string]any{
		auditMetaSource:         "cli",
		auditMetaProjectRoot:    projectRoot,
		auditMetaCommand:        metric.Command,
		auditMetaNormalizedCmd:  metric.NormalizedCmd,
		auditMetaArgs:           metric.Args,
		auditMetaFlags:          metric.Flags,
		auditMetaDurationMs:     metric.Duration.Milliseconds(),
		auditMetaStartTime:      metric.StartTime.Format(time.RFC3339),
		auditMetaEndTime:        metric.EndTime.Format(time.RFC3339),
		auditMetaExitCode:       metric.ExitCode,
		auditMetaTimedOut:       metric.TimedOut,
		auditMetaObjectsCreated: metric.ObjectsCreated,
		auditMetaObjectsUpdated: metric.ObjectsUpdated,
		auditMetaObjectsDeleted: metric.ObjectsDeleted,
	}

	addContextMetadata(metadata, metric)
	addActorMetadata(metadata, metric)
	addErrorMetadata(metadata, metric)

	return metadata
}

// addContextMetadata adds context information to metadata
func addContextMetadata(metadata map[string]any, metric *clitool.CommandMetric) {
	if metric.PriorityPlan != EmptyValue {
		metadata[auditMetaPriorityPlan] = metric.PriorityPlan
	}
	if metric.Workstream != EmptyValue {
		metadata[auditMetaWorkstream] = metric.Workstream
	}
	if metric.Milestone != EmptyValue {
		metadata[auditMetaMilestone] = metric.Milestone
	}
}

// addActorMetadata adds actor information to metadata
func addActorMetadata(metadata map[string]any, metric *clitool.CommandMetric) {
	if metric.ActorID != EmptyValue {
		metadata[auditMetaActorID] = metric.ActorID
		metadata[auditMetaActorRoles] = metric.ActorRoles
	}
}

// addErrorMetadata adds error information to metadata
func addErrorMetadata(metadata map[string]any, metric *clitool.CommandMetric) {
	if metric.Error != EmptyValue {
		metadata[auditMetaError] = metric.Error
	}
}

// validateAndWriteAuditEvent writes the audit event using the coordination system when
// storage is already available (ignorable operation: only use, never create).
// Call only when cli.StorageAvailableForOptionalUse(cmd) is true; otherwise skipped.
func validateAndWriteAuditEvent(cmd *cobra.Command, projectRoot string, operation string, severity string, metadata map[string]any, metric *clitool.CommandMetric, profile string) error {
	if !cli.StorageAvailableForOptionalUse(cmd) {
		return nil // No active storage service — skip without creating storage
	}
	p := cli.GetStorageProvider(cmd.Context())
	if p == nil {
		return nil
	}
	fileStorage, ok := p.(storage.ObjectStorageProvider)
	if !ok {
		return nil
	}
	ctx := pkgctx.NewSystemContext()
	emitCommandExecutionEventViaCoordinator(ctx, projectRoot, fileStorage, operation, severity, metadata, metric, profile)
	return nil
}
