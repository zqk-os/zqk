package scheduler

import (
	"context"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/scheduler/transceiver"
	"github.com/zqk-os/zqk/pkg/scheduler/transceiver/types"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// CreateMessageFromJob creates a Message from scheduler job execution
func CreateMessageFromJob(eventType string, job *ScheduledJob, payload map[string]any) types.Message {
	if payload == nil {
		payload = make(map[string]any)
	}

	metadata := map[string]string{
		KeyJobID:       job.ID,
		KeyJobType:     job.JobType,
		KeyCategory:    job.Category,
		KeyTitle:       job.Title,
		KeyDescription: job.Description,
	}

	if payload[KeySeverity] != nil {
		if severity, ok := payload[KeySeverity].(string); ok {
			metadata[KeySeverity] = severity
		}
	}

	return types.Message{
		EventType: eventType,
		Source:    "scheduler",
		Timestamp: time.Now().UTC(),
		Payload:   payload,
		Metadata:  metadata,
	}
}

// CreateCompletionMessage creates a completion message for a job
func CreateCompletionMessage(job *ScheduledJob, duration time.Duration, stdout, stderr string, exitCode int) types.Message {
	payload := map[string]any{
		KeyJobID:     job.ID,
		KeyJobType:   job.JobType,
		KeyCategory:  job.Category,
		KeyCommand:   job.Command,
		KeyDuration:  duration.Seconds(),
		KeySuccess:   exitCode == 0,
		KeyExitCode:  exitCode,
		KeyTimestamp: zqktime.NowRFC3339UTC(),
	}

	if stdout != emptyValue {
		payload[KeyStdout] = stdout
	}
	if stderr != emptyValue {
		payload[KeyStderr] = stderr
	}

	return CreateMessageFromJob("scheduler_job_completed", job, payload)
}

// CreateErrorMessage creates an error message for a failed job.
// failureKind and failureReason are unambiguous classifications (e.g. "timeout", "shell_exec", "script_exit") so consumers never have to guess.
func CreateErrorMessage(job *ScheduledJob, duration time.Duration, errorMsg, stderr string, attempts, exitCode int, failureKind, failureReason string) types.Message {
	payload := map[string]any{
		KeyJobID:         job.ID,
		KeyJobType:       job.JobType,
		KeyCategory:      job.Category,
		KeyCommand:       job.Command,
		KeyDuration:      duration.Seconds(),
		KeySuccess:       false,
		KeyFailureKind:   failureKind,
		KeyFailureReason: failureReason,
		KeyError:         errorMsg,
		KeyAttempts:      attempts,
		KeyExitCode:      exitCode,
		KeyTimestamp:     zqktime.NowRFC3339UTC(),
		KeySeverity:      "high", // Job failures are high severity
	}

	if stderr != emptyValue {
		payload[KeyStderr] = stderr
	}

	return CreateMessageFromJob("scheduler_job_failed", job, payload)
}

// CreateStatusMessage creates a status update message for a job
func CreateStatusMessage(job *ScheduledJob, status string, progress any) types.Message {
	payload := map[string]any{
		KeyJobID:     job.ID,
		KeyStatus:    status,
		KeyTimestamp: zqktime.NowRFC3339UTC(),
	}

	if progress != nil {
		payload[KeyProgress] = progress
	}

	return CreateMessageFromJob("scheduler_job_status", job, payload)
}

// CreateCallbackMessage creates a callback message for routing through transceiver
// callbackType should be "completion", "error", or "status"
func CreateCallbackMessage(job *ScheduledJob, callbackType string, payload map[string]any) types.Message {
	if payload == nil {
		payload = make(map[string]any)
	}

	// Ensure required fields are set
	if payload[KeyJobID] == nil {
		payload[KeyJobID] = job.ID
	}
	if payload[KeyTimestamp] == nil {
		payload[KeyTimestamp] = zqktime.NowRFC3339UTC()
	}

	// Set callback type in payload
	payload[KeyCallbackType] = callbackType

	// Determine event type based on callback type
	eventType := "scheduler_job_callback"
	switch callbackType {
	case "completion":
		eventType = "scheduler_job_callback_completion"
	case "error":
		eventType = "scheduler_job_callback_error"
	case "status":
		eventType = "scheduler_job_callback_status"
	}

	return CreateMessageFromJob(eventType, job, payload)
}

// RouteJobMessageAsync routes a message asynchronously using AsyncRouter
// Non-blocking; silently skips if async router is nil
//
//nolint:gocritic // Message passed by value to avoid shared mutation
func RouteJobMessageAsync(ctx context.Context, asyncRouter *transceiver.AsyncRouter, message types.Message) error {
	if asyncRouter == nil {
		return nil
	}
	err := asyncRouter.RouteAsync(ctx, message)
	if err != nil {
		// No-Op handler pattern: instead of propagating error to diagnostics.jsonl which causes failure storms,
		// we treat this as a No-Op routing event. Log it clearly as an audit diagnostic, not a system failure.
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
			Warn("scheduler_transceiver_router_routing_no_op").
			EventType(message.EventType).
			WithError(err).
			Log()
		return nil // Return nil so scheduler daemon does not treat this as a failed job execution
	}
	return nil
}
