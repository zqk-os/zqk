package asynccheck

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/metrics"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// FormatProgressSummary returns a standardized human-readable progress line.
func FormatProgressSummary(percent float64, progress, totalTasks, queueSize int, ratePerSec float64, phase string, workers, goroutines int, elapsed time.Duration) string {
	if totalTasks <= 0 || math.IsNaN(percent) {
		percent = 0.0
	}
	if percent > 100.0 {
		percent = 100.0
	}

	const barWidth = 40
	completedBars := int(percent / 100.0 * float64(barWidth))
	if completedBars < 0 {
		completedBars = 0
	}
	if completedBars > barWidth {
		completedBars = barWidth
	}

	bar := make([]byte, barWidth)
	for i := 0; i < completedBars; i++ {
		bar[i] = '='
	}
	for i := completedBars; i < barWidth; i++ {
		bar[i] = ' '
	}

	phaseStr := ""
	if phase != "" {
		phaseStr = fmt.Sprintf(" (%s, workers: %d, goroutines: %d)", phase, workers, goroutines)
	}

	return fmt.Sprintf("Progress: [%s] %.1f%% (%d/%d, queue: %d) %.0f/s%s - %s",
		string(bar), percent, progress, totalTasks, queueSize, ratePerSec, phaseStr, FormatDuration(elapsed))
}

// CalculateRemainingTime estimates remaining duration based on current rate.
func CalculateRemainingTime(total, completed int, ratePerSec float64) time.Duration {
	if ratePerSec <= 0 || completed >= total {
		return 0
	}
	remainingTasks := total - completed
	remainingSeconds := float64(remainingTasks) / ratePerSec
	return time.Duration(remainingSeconds * float64(time.Second))
}

// FormatDuration formats duration in a compact, human-readable string.
func FormatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	switch {
	case h > 0:
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	case m > 0:
		return fmt.Sprintf("%02d:%02d", m, s)
	default:
		return fmt.Sprintf("%ds", s)
	}
}

// EmitCheckProgressEventViaCoordinator emits system check progress events via the coordination system.
func EmitCheckProgressEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operationID string,
	eventType string,
	progress int,
	totalTasks int,
	completed int,
	failed int,
	queueSize int,
	message string,
	profile string,
) {
	if projectRoot == "" {
		return
	}

	coordinator := newCoordinatorForProject(projectRoot, storageProvider)
	percent := calculatePercent(progress, totalTasks)

	loggingFields := buildProgressLoggingFields(operationID, eventType, message, progress, totalTasks, completed, failed, queueSize, percent)
	auditMetadata := buildProgressAuditMetadata(eventType, progress, totalTasks, completed, failed, queueSize, percent)

	metricsData := map[string]any{
		"progress":                 progress,
		"total_tasks":              totalTasks,
		objects.FieldKeyPercentComplete: percent,
		"completed":                completed,
		objects.FieldKeyFailed:     failed,
		"queue_size":               queueSize,
	}

	status := objects.ObjectStatusSuccess
	sev := SeverityLow
	if eventType == ProgressEventTypeTimeout || eventType == ProgressEventTypeStuck {
		status = "degraded"
		sev = SeverityMedium
	}

	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: auditMetadata,
		MetricsData:   metricsData,
	}

	eventCtx := coordination.NewEventContext(operationID, "system_check", status).
		WithLevel(sev).
		WithEventData(eventData).
		WithContext(ctx)

	switch eventType {
	case ProgressEventTypeProgress:
		eventCtx = eventCtx.WithChannels(true, false, true, false)
	case ProgressEventTypeSummary:
		eventCtx = eventCtx.WithChannels(true, true, true, false)
	case ProgressEventTypeCompleted:
		eventCtx = eventCtx.WithChannels(true, true, true, true)
	default:
		eventCtx = eventCtx.WithChannels(true, true, true, false)
	}

	if emitErr := coordinator.Emit(ctx, eventCtx); emitErr != nil {
		// Logged or handled by caller
	}
}

func newCoordinatorForProject(projectRoot string, storageProvider storage.ObjectStorageProvider) *coordination.Coordinator {
	auditRouter := coordination.StorageAuditRouterForProject(projectRoot, storageProvider)
	var metricsRouter coordination.MetricsRouter
	if storageProvider != nil {
		metricsPipeline := metrics.MetricPipelineForProject(storageProvider, projectRoot)
		metricsRouter = coordination.NewMetricPipelineRouter(metricsPipeline)
	}

	return coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		AuditRouter:       auditRouter,
		MetricsRouter:     metricsRouter,
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
}

func calculatePercent(progress, totalTasks int) float64 {
	if totalTasks <= 0 {
		return 0.0
	}
	percent := float64(progress) / float64(totalTasks) * 100.0
	if percent > 100.0 {
		return 100.0
	}
	return percent
}

func buildProgressLoggingFields(operationID, eventType, message string, progress, totalTasks, completed, failed, queueSize int, percent float64) []coordination.LoggingField {
	fields := []coordination.LoggingField{
		{Key: "operation_id", Value: operationID},
		{Key: objects.FieldKeyPhase, Value: eventType},
		{Key: "progress", Value: progress},
		{Key: "total_tasks", Value: totalTasks},
		{Key: "percent_complete", Value: percent},
		{Key: "completed", Value: completed},
		{Key: "failed", Value: failed},
		{Key: "queue_size", Value: queueSize},
	}
	if message != "" {
		fields = append(fields, coordination.LoggingField{Key: "message", Value: message})
	}
	return fields
}

func buildProgressAuditMetadata(eventType string, progress, totalTasks, completed, failed, queueSize int, percent float64) map[string]any {
	shouldAudit := eventType == ProgressEventTypeSummary || eventType == ProgressEventTypeCompleted ||
		eventType == ProgressEventTypeTimeout || eventType == ProgressEventTypeStuck
	if !shouldAudit {
		return nil
	}

	sev := SeverityLow
	if eventType == ProgressEventTypeTimeout || eventType == ProgressEventTypeStuck {
		sev = SeverityMedium
	}

	return map[string]any{
		objects.FieldKeyEventType:       "system_config_change",
		objects.FieldKeyOperation:       fmt.Sprintf("System check %s: %d/%d objects (%.1f%%)", eventType, progress, totalTasks, percent),
		objects.FieldKeySeverity:        sev,
		"progress":                      progress,
		"total_tasks":                   totalTasks,
		objects.FieldKeyPercentComplete: percent,
		"completed":                     completed,
		objects.FieldKeyFailed:          failed,
		"queue_size":                    queueSize,
	}
}
