package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/scheduler/transceiver"
	"github.com/zqk-os/zqk/pkg/shellcmd"
	"github.com/zqk-os/zqk/pkg/when"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

const (
	callbackTypeCompletion = "completion"
	callbackTypeError      = "error"
	callbackTypeStatus     = "status"
	callbackMechanismHook  = "webhook"
	callbackMechanismCmd   = "command"
	callbackMechanismEvent = "event"
	callbackURLField       = "callback_url"
	callbackMechanismField = "callback_mechanism"
	callbackEventNameField = "event_name"
	callbackTypeField      = "callback_type"
	webhookURLPrefixHTTP   = "http://"
	webhookURLPrefixHTTPS  = "https://"
	webhookMethodPOST      = "POST"
	httpHeaderContentType  = "Content-Type"
	httpHeaderUserAgent    = "User-Agent"
	contentTypeJSON        = "application/json"
	schedulerUserAgent     = "ZQK-Scheduler/1.0"
	callbackFieldCommand   = "command"
	callbackFieldURL       = "url"
	callbackFieldStatus    = "status_code"
	callbackFieldResponse  = "response"
	callbackFieldPayload   = "payload"
	callbackFieldStdout    = "stdout"
	callbackFieldStderr    = "stderr"
	callbackTimeoutWebhook = 5 * time.Second
	callbackTimeoutCmd     = 10 * time.Second
	callbackBodyReadLimit  = 1024
	callbackHTTPStatusMin  = 200
	callbackHTTPStatusMax  = 300
)

// executeCallback executes a callback hook (webhook, command, or event)
// Callbacks are routed through the transceiver async router for queued, non-blocking execution
func (h *RunWrapperHandler) executeCallback(ctx context.Context, job *ScheduledJob, callbackType string, payload map[string]any) {
	var callbackURL string
	switch callbackType {
	case callbackTypeCompletion:
		callbackURL = job.CallbackOnCompletion
	case callbackTypeError:
		callbackURL = job.CallbackOnError
	case callbackTypeStatus:
		callbackURL = job.CallbackOnStatus
	default:
		return
	}

	if callbackURL == emptyValue {
		return
	}

	// Determine callback mechanism
	cbType := job.CallbackType
	if cbType == emptyValue {
		when.When(func() bool {
			return strings.HasPrefix(callbackURL, webhookURLPrefixHTTP) || strings.HasPrefix(callbackURL, webhookURLPrefixHTTPS)
		}).Then(func() { cbType = callbackMechanismHook }).OrElse(func() { cbType = callbackMechanismCmd }).Run()
	}

	// Add job metadata to payload
	if payload == nil {
		payload = make(map[string]any)
	}
	payload[KeyJobID] = job.ID
	payload[KeyJobType] = job.JobType
	payload[KeyCategory] = job.Category
	payload[KeyTimestamp] = zqktime.NowRFC3339UTC()
	payload[callbackURLField] = callbackURL
	payload[callbackMechanismField] = cbType

	// Route callback through transceiver async router for queued, non-blocking execution
	// This allows callbacks to be processed asynchronously and provides:
	// - Queuing for load handling
	// - Worker pool for concurrent processing
	// - Retry logic via routing rules
	// - Monitoring via router metrics
	when.When(func() bool { return h.asyncRouter != nil }).Then(func() {
		callbackMsg := CreateCallbackMessage(job, callbackType, payload)
		err := RouteJobMessageAsync(ctx, h.asyncRouter, callbackMsg)
		when.When(func() bool { return err == nil }).Then(func() {
			RunWrapperLog(h.logger).Debug(LogEventRunWrapperCallbackRoutedThroughTransceiver).
				JobID(job.ID).
				String(KeyCallbackType, callbackType).
				String("mechanism", cbType).
				Log()
		}).OrElse(func() {
			RunWrapperLog(h.logger).Warn(LogEventRunWrapperCallbackTransceiverRouteFailed).
				WithFields(append([]logging.Field{logging.String(KeyJobID, job.ID), logging.String(KeyCallbackType, callbackType)}, logErrField(err)...)...).
				Log()
			h.executeCallbackDirect(ctx, job, cbType, callbackURL, payload)
		}).Run()
	}).OrElse(func() {
		// No async router available, execute directly (backward compatibility)
		h.executeCallbackDirect(ctx, job, cbType, callbackURL, payload)
	}).Run()
}

// executeCallbackDirect executes a callback directly (synchronous execution)
// This is used as a fallback when transceiver routing is unavailable or fails
func (h *RunWrapperHandler) executeCallbackDirect(ctx context.Context, job *ScheduledJob, cbType, callbackURL string, payload map[string]any) {
	switch cbType {
	case callbackMechanismHook:
		h.executeWebhookCallback(ctx, callbackURL, payload)
	case callbackMechanismCmd:
		h.executeCommandCallback(ctx, callbackURL, payload)
	case callbackMechanismEvent:
		h.executeEventCallback(ctx, callbackURL, payload)
	default:
		RunWrapperLog(h.logger).Warn(LogEventRunWrapperCallbackUnknownMechanism).
			JobID(job.ID).
			String(KeyCallbackType, cbType).
			String(callbackURLField, callbackURL).
			Log()
	}
}

// executeWebhookCallback executes an HTTP webhook callback
func (h *RunWrapperHandler) executeWebhookCallback(ctx context.Context, url string, payload map[string]any) {
	// Create timeout context for webhook (5 seconds max)
	webhookCtx, cancel := context.WithTimeout(ctx, callbackTimeoutWebhook)
	defer cancel()

	// Marshal payload to JSON
	jsonData, err := json.Marshal(payload)
	if err != nil {
		RunWrapperLog(h.logger).Warn(LogEventRunWrapperCallbackMarshalWebhookPayloadFailed).
			WithFields(append([]logging.Field{logging.String(callbackFieldURL, url)}, logErrField(err)...)...).
			Log()
		return
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(webhookCtx, webhookMethodPOST, url, bytes.NewBuffer(jsonData))
	if err != nil {
		RunWrapperLog(h.logger).Warn(LogEventRunWrapperCallbackCreateWebhookRequestFailed).
			String(callbackFieldURL, url).
			WithError(err).
			Log()
		return
	}

	req.Header.Set(httpHeaderContentType, contentTypeJSON)
	req.Header.Set(httpHeaderUserAgent, schedulerUserAgent)

	// Execute request
	client := &http.Client{
		Timeout: callbackTimeoutWebhook,
	}
	resp, err := client.Do(req)
	if err != nil {
		RunWrapperLog(h.logger).Warn(LogEventRunWrapperCallbackWebhookRequestFailed).
			WithFields(append([]logging.Field{logging.String(callbackFieldURL, url)}, logErrField(err)...)...).
			Log()
		return
	}
	defer resp.Body.Close()

	// Read response body (limit to 1KB to avoid memory issues)
	body, errRead := io.ReadAll(io.LimitReader(resp.Body, callbackBodyReadLimit))
	if errRead != nil {
		RunWrapperLog(h.logger).Debug("Failed to read webhook response body").WithError(errRead).Log()
	}

	when.When(func() bool {
		return resp.StatusCode >= callbackHTTPStatusMin && resp.StatusCode < callbackHTTPStatusMax
	}).Then(func() {
		RunWrapperLog(h.logger).Debug(LogEventRunWrapperCallbackWebhookSucceeded).
			String(callbackFieldURL, url).
			Int(callbackFieldStatus, resp.StatusCode).
			Log()
	}).OrElse(func() {
		RunWrapperLog(h.logger).Warn(LogEventRunWrapperCallbackWebhookNon2xx).
			String(callbackFieldURL, url).
			Int(callbackFieldStatus, resp.StatusCode).
			String(callbackFieldResponse, string(body)).
			Log()
	}).Run()
}

// executeCommandCallback executes a command callback
func (h *RunWrapperHandler) executeCommandCallback(ctx context.Context, command string, payload map[string]any) {
	runCommandCallback(ctx, h.logger, h.getExecutor(), command, payload)
}

// runCommandCallback executes a callback command with the JSON payload on stdin.
// Callback strings are authored as shell one-liners, so shellcmd.Argv decides
// whether the string needs a shell instead of being split on whitespace.
func runCommandCallback(ctx context.Context, logger logging.Logger, executor CommandExecutor, command string, payload map[string]any) {
	// Create timeout context for callback (10 seconds max)
	callbackCtx, cancel := context.WithTimeout(ctx, callbackTimeoutCmd)
	defer cancel()

	jsonData, err := json.Marshal(payload)
	if err != nil {
		RunWrapperLog(logger).Warn(LogEventRunWrapperCallbackMarshalCommandPayloadFailed).
			WithFields(append([]logging.Field{logging.String(callbackFieldCommand, command)}, logErrField(err)...)...).
			Log()
		return
	}

	argv := shellcmd.Argv(command)
	if len(argv) == 0 {
		RunWrapperLog(logger).Warn(LogEventRunWrapperCallbackEmptyCommand).
			String(callbackFieldCommand, command).
			Log()
		return
	}

	//nolint:gosec // G204: Command execution is intentional - this is a command callback handler that executes user-defined commands
	cmd := executor.CommandContext(callbackCtx, argv[0], argv[1:]...)
	cmd.SetStdin(bytes.NewReader(jsonData))

	var stdout, stderr bytes.Buffer
	cmd.SetStdout(&stdout)
	cmd.SetStderr(&stderr)

	if err := cmd.Run(); err != nil {
		RunWrapperLog(logger).Warn(LogEventRunWrapperCallbackCommandFailed).
			WithFields(append([]logging.Field{logging.String(callbackFieldCommand, command), logging.String(callbackFieldStderr, stderr.String())}, logErrField(err)...)...).
			Log()
		return
	}

	RunWrapperLog(logger).Debug(LogEventRunWrapperCallbackCommandSucceeded).
		String(callbackFieldCommand, command).
		String(callbackFieldStdout, stdout.String()).
		Log()
}

// executeEventCallback emits a system event (placeholder for future event system)
func (h *RunWrapperHandler) executeEventCallback(_ context.Context, eventName string, payload map[string]any) {
	// This would integrate with a future event bus/system
	// For now, just log it
	RunWrapperLog(h.logger).Info(LogEventRunWrapperCallbackEventNotImplemented).
		String(callbackEventNameField, eventName).
		String(callbackFieldPayload, fmt.Sprintf("%v", payload)).
		Log()
}

// InvokeJobCallback invokes the job's completion or error callback (webhook or command).
// Used by the scheduler after any job type (e.g. retention_tolerance) completes or fails,
// so callbacks can chain (e.g. update job KINDS and trigger next kind).
// logger and asyncRouter can be nil; if asyncRouter is nil, callback runs directly.
func InvokeJobCallback(ctx context.Context, logger logging.Logger, asyncRouter *transceiver.AsyncRouter, job *ScheduledJob, callbackType string, payload map[string]any) {
	var callbackURL string
	switch callbackType {
	case callbackTypeCompletion:
		callbackURL = job.CallbackOnCompletion
	case callbackTypeError:
		callbackURL = job.CallbackOnError
	case callbackTypeStatus:
		callbackURL = job.CallbackOnStatus
	default:
		return
	}
	if callbackURL == emptyValue {
		return
	}
	cbType := job.CallbackType
	if cbType == emptyValue {
		when.When(func() bool {
			return strings.HasPrefix(callbackURL, webhookURLPrefixHTTP) || strings.HasPrefix(callbackURL, webhookURLPrefixHTTPS)
		}).Then(func() {
			cbType = callbackMechanismHook
		}).OrElse(func() {
			cbType = callbackMechanismCmd
		}).Run()
	}
	if payload == nil {
		payload = make(map[string]any)
	}
	payload[KeyJobID] = job.ID
	payload[KeyJobType] = job.JobType
	payload[KeyCategory] = job.Category
	payload[KeyTimestamp] = zqktime.NowRFC3339UTC()
	payload[callbackURLField] = callbackURL
	payload[callbackMechanismField] = cbType
	// Include env for retention sequential: callback can see KINDS just processed
	if len(job.EnvironmentVariables) > 0 {
		payload[objects.FieldKeyEnvironmentVariables] = job.EnvironmentVariables
	}
	// Command callbacks are completion evidence, not advisory messages. Execute
	// them synchronously so a healthy async router cannot accept and then lose
	// the callback before any consumer writes its artifact.
	if cbType == callbackMechanismCmd {
		executeCallbackDirectWithLogger(ctx, logger, cbType, callbackURL, payload)
		return
	}
	when.When(func() bool { return asyncRouter != nil }).Then(func() {
		callbackMsg := CreateCallbackMessage(job, callbackType, payload)
		if routeErr := RouteJobMessageAsync(ctx, asyncRouter, callbackMsg); routeErr != nil {
			if logger != nil {
				RunWrapperLog(logger).Warn(LogEventRunWrapperCallbackInvokeRouteFailedExecutingDirect).
					JobID(job.ID).
					String(KeyCallbackType, callbackType).
					WithError(routeErr).
					Log()
			}
			executeCallbackDirectWithLogger(ctx, logger, cbType, callbackURL, payload)
		}
	}).OrElse(func() {
		executeCallbackDirectWithLogger(ctx, logger, cbType, callbackURL, payload)
	}).Run()
}

// executeCallbackDirectWithLogger runs webhook/command/event callback with a given logger (for use from InvokeJobCallback).
func executeCallbackDirectWithLogger(ctx context.Context, logger logging.Logger, cbType, callbackURL string, payload map[string]any) {
	if logger == nil {
		return
	}
	switch cbType {
	case callbackMechanismHook:
		executeWebhookWithLogger(ctx, logger, callbackURL, payload)
	case callbackMechanismCmd:
		executeCommandWithLogger(ctx, logger, callbackURL, payload)
	case callbackMechanismEvent:
		RunWrapperLog(logger).Info(LogEventRunWrapperCallbackEventNotImplemented).
			String(callbackEventNameField, callbackURL).
			String(callbackFieldPayload, fmt.Sprintf("%v", payload)).
			Log()
	default:
		jobID, _ := payload[KeyJobID].(string)
		RunWrapperLog(logger).Warn(LogEventRunWrapperCallbackUnknownMechanism).
			JobID(jobID).
			String(callbackTypeField, cbType).
			String(callbackURLField, callbackURL).
			Log()
	}
}

func executeWebhookWithLogger(ctx context.Context, logger logging.Logger, url string, payload map[string]any) {
	webhookCtx, cancel := context.WithTimeout(ctx, callbackTimeoutWebhook)
	defer cancel()
	jsonData, err := json.Marshal(payload)
	if err != nil {
		RunWrapperLog(logger).Warn(LogEventRunWrapperCallbackMarshalWebhookPayloadFailed).
			String(callbackFieldURL, url).
			WithError(err).
			Log()
		return
	}
	req, err := http.NewRequestWithContext(webhookCtx, webhookMethodPOST, url, bytes.NewBuffer(jsonData))
	if err != nil {
		RunWrapperLog(logger).Warn(LogEventRunWrapperCallbackCreateWebhookRequestFailed).
			String(callbackFieldURL, url).
			WithError(err).
			Log()
		return
	}
	req.Header.Set(httpHeaderContentType, contentTypeJSON)
	req.Header.Set(httpHeaderUserAgent, schedulerUserAgent)
	client := &http.Client{Timeout: callbackTimeoutWebhook}
	resp, err := client.Do(req)
	if err != nil {
		RunWrapperLog(logger).Warn(LogEventRunWrapperCallbackWebhookRequestFailed).
			String(callbackFieldURL, url).
			WithError(err).
			Log()
		return
	}
	defer resp.Body.Close()
	body, errRead := io.ReadAll(io.LimitReader(resp.Body, callbackBodyReadLimit))
	if errRead != nil {
		RunWrapperLog(logger).Debug("Failed to read webhook response body").WithError(errRead).Log()
	}
	when.When(func() bool {
		return resp.StatusCode >= callbackHTTPStatusMin && resp.StatusCode < callbackHTTPStatusMax
	}).Then(func() {
		RunWrapperLog(logger).Debug(LogEventRunWrapperCallbackWebhookSucceeded).
			String(callbackFieldURL, url).
			Int(callbackFieldStatus, resp.StatusCode).
			Log()
	}).OrElse(func() {
		RunWrapperLog(logger).Warn(LogEventRunWrapperCallbackWebhookNon2xx).
			String(callbackFieldURL, url).
			Int(callbackFieldStatus, resp.StatusCode).
			String(callbackFieldResponse, string(body)).
			Log()
	}).Run()
}

func executeCommandWithLogger(ctx context.Context, logger logging.Logger, command string, payload map[string]any) {
	runCommandCallback(ctx, logger, &NativeExecutor{}, command, payload)
}
