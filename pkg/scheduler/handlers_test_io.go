package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/scheduler/transceiver"
	"github.com/zqk-os/zqk/pkg/scheduler/transceiver/types"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

const (
	errAsyncRouterUnavailable  = "async router not available - cannot test routing channels"
	errTestMessageFailedFmt    = "test message failed: %w"
	logLevelDefault            = "default"
	logLevelVerbose            = "verbose"
	testMessageEventType       = "test_io_message"
	testLogLevelEventType      = "test_io_log_level"
	messageSeverityLow         = "low"
	testMessageSourceScheduler = "scheduler"
)

// TestIOHandler tests transceiver routing channels by sending test messages
type TestIOHandler struct {
	storage     storagepkg.ObjectStorageProvider
	logger      logging.Logger
	asyncRouter *transceiver.AsyncRouter
}

// NewTestIOHandler creates a new test I/O handler
// NewTestIOHandler creates a new test IO handler
func NewTestIOHandler(storage storagepkg.ObjectStorageProvider, logger logging.Logger, asyncRouter *transceiver.AsyncRouter) TestIOHandlerInterface {
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}
	return &TestIOHandler{
		storage:     storage,
		logger:      logger,
		asyncRouter: asyncRouter,
	}
}

// Execute sends test messages through configured routing channels
func (h *TestIOHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	return RunTestIOViaPipeline(ctx, h, job)
}

// executeTestIOCore sends test messages through configured routing channels.
// Called from RunTestIOViaPipeline NORMALIZE stage.
func (h *TestIOHandler) executeTestIOCore(ctx context.Context, job *ScheduledJob) error {
	TestIOLog(h.logger).Info(LogEventTestIOJobStart).
		JobID(job.ID).
		Log()

	if h.asyncRouter == nil {
		return errors.New(errAsyncRouterUnavailable)
	}

	// Get test configuration from environment variables
	testConfig := h.getTestConfig(job)

	// If testing log levels, add log level test messages
	if testConfig.TestLogLevels {
		logLevelMessages := h.createLogLevelTestMessages(job)
		testConfig.Messages = append(logLevelMessages, testConfig.Messages...)
	}

	// Send test messages
	results := make([]TestIOResult, 0, len(testConfig.Messages))

	for i, msgConfig := range testConfig.Messages {
		result := h.sendTestMessage(ctx, job, msgConfig, i+1)
		results = append(results, result)

		// Small delay between messages to avoid overwhelming the router
		if i < len(testConfig.Messages)-1 {
			time.Sleep(testConfig.MessageDelay)
		}
	}

	// Log summary with appropriate detail based on job log level
	switch job.LogLevel {
	case string(pkgctx.ProfileDebug):
		TestIOLog(h.logger).Info(LogEventTestIOJobCompleted).
			JobID(job.ID).
			MessagesSent(len(results)).
			MessagesSucceeded(countSuccess(results)).
			MessagesFailed(countFailures(results)).
			String("test_log_levels", fmt.Sprintf("%v", testConfig.TestLogLevels)).
			String("message_delay", testConfig.MessageDelay.String()).
			String("fail_on_error", fmt.Sprintf("%v", testConfig.FailOnError)).
			Log()
	case logLevelVerbose:
		TestIOLog(h.logger).Info(LogEventTestIOJobCompleted).
			JobID(job.ID).
			MessagesSent(len(results)).
			MessagesSucceeded(countSuccess(results)).
			MessagesFailed(countFailures(results)).
			String("test_log_levels", fmt.Sprintf("%v", testConfig.TestLogLevels)).
			Log()
	default:
		TestIOLog(h.logger).Info(LogEventTestIOJobCompleted).
			JobID(job.ID).
			MessagesSent(len(results)).
			MessagesSucceeded(countSuccess(results)).
			MessagesFailed(countFailures(results)).
			Log()
	}

	// If any messages failed and fail_on_error is true, return error
	if testConfig.FailOnError {
		for _, result := range results {
			if result.Error != nil {
				return errfmt.Errorf(errTestMessageFailedFmt, result.Error)
			}
		}
	}

	return nil
}

// TestConfig holds configuration for test I/O job
type TestConfig struct {
	Messages            []TestMessageConfig
	MessageDelay        time.Duration
	FailOnError         bool
	VerifyRouting       bool          // If true, verify that messages were actually routed
	TestLogLevels       bool          // If true, test all log levels (default, verbose, debug)
	VerificationTimeout time.Duration // Timeout for verification (default: 30s)
}

// TestMessageConfig defines a test message to send
type TestMessageConfig struct {
	EventType string            `json:"event_type"`
	Severity  string            `json:"severity,omitempty"`
	Payload   map[string]any    `json:"payload,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// TestIOResult holds the result of sending a test message
type TestIOResult struct {
	MessageIndex int
	EventType    string
	Success      bool
	Error        error
	Timestamp    time.Time
}

// getTestConfig extracts test configuration from job environment variables
func (h *TestIOHandler) getTestConfig(job *ScheduledJob) TestConfig {
	config := TestConfig{
		Messages:            []TestMessageConfig{},
		MessageDelay:        100 * time.Millisecond,
		FailOnError:         false,
		VerifyRouting:       false,
		VerificationTimeout: 30 * time.Second,
	}

	if job.EnvironmentVariables == nil {
		// Default: send a simple test message
		config.Messages = []TestMessageConfig{
			{
				EventType: "test_io_message",
				Severity:  messageSeverityLow,
				Payload: map[string]any{
					"test":                 true,
					"job_id":               job.ID,
					"timestamp":            zqktime.NowRFC3339UTC(),
					"message":              "Test I/O job verification",
					objects.FieldKeySource: testMessageSourceScheduler,
				},
			},
		}
		return config
	}

	// Parse message delay
	if delayStr, ok := job.EnvironmentVariables[EnvKeyTestIOMessageDelay]; ok && delayStr != emptyValue {
		if duration, err := time.ParseDuration(delayStr); err == nil {
			config.MessageDelay = duration
		}
	}

	// Parse fail_on_error
	if failStr, ok := job.EnvironmentVariables[EnvKeyTestIOFailOnError]; ok && failStr != emptyValue {
		config.FailOnError = failStr == "true" || failStr == "1"
	}

	// Parse verify_routing
	if verifyStr, ok := job.EnvironmentVariables[EnvKeyTestIOVerifyRouting]; ok && verifyStr != emptyValue {
		config.VerifyRouting = verifyStr == "true" || verifyStr == "1"
	}

	// Parse test_log_levels (default: true if TEST_MESSAGES not specified)
	if testLogLevelsStr, ok := job.EnvironmentVariables[EnvKeyTestIOTestLogLevels]; ok && testLogLevelsStr != emptyValue {
		config.TestLogLevels = testLogLevelsStr == "true" || testLogLevelsStr == "1"
	} else {
		// Default to true if no custom messages specified
		_, hasCustomMessages := job.EnvironmentVariables[EnvKeyTestIOTestMessages]
		config.TestLogLevels = !hasCustomMessages
	}

	// Parse test messages from JSON
	if messagesJSON, ok := job.EnvironmentVariables[EnvKeyTestIOTestMessages]; ok && messagesJSON != emptyValue {
		var messages []TestMessageConfig
		if err := json.Unmarshal([]byte(messagesJSON), &messages); err == nil {
			config.Messages = messages
		} else {
			TestIOLog(h.logger).Warn(LogEventTestIOTestMessagesParseFailed).
				WithFields(jobLogFieldsWithErr(job, err)...).
				Log()
			// Fall back to default
			config.Messages = []TestMessageConfig{
				{
					EventType: "test_io_message",
					Severity:  messageSeverityLow,
					Payload: map[string]any{
						"test":      true,
						"job_id":    job.ID,
						"timestamp": zqktime.NowRFC3339UTC(),
					},
				},
			}
		}
	} else {
		// Default: send a simple test message
		config.Messages = []TestMessageConfig{
			{
				EventType: "test_io_message",
				Severity:  "low",
				Payload: map[string]any{
					"test":      true,
					"job_id":    job.ID,
					"timestamp": zqktime.NowRFC3339UTC(),
					"message":   "Test I/O job verification",
				},
			},
		}
	}

	return config
}

// sendTestMessage sends a single test message through the router
func (h *TestIOHandler) sendTestMessage(ctx context.Context, job *ScheduledJob, msgConfig TestMessageConfig, index int) TestIOResult {
	result := TestIOResult{
		MessageIndex: index,
		EventType:    msgConfig.EventType,
		Timestamp:    time.Now().UTC(),
	}

	// Build message payload
	payload := make(map[string]any)
	if msgConfig.Payload != nil {
		maps.Copy(payload, msgConfig.Payload)
	}

	// Add default fields
	payload["test_message_index"] = index
	payload["test_timestamp"] = result.Timestamp.Format(time.RFC3339)
	payload["job_id"] = job.ID

	// Build metadata
	metadata := make(map[string]string)
	if msgConfig.Metadata != nil {
		maps.Copy(metadata, msgConfig.Metadata)
	}

	// Add default metadata
	metadata["job_id"] = job.ID
	metadata[objects.FieldKeyJobType] = job.JobType
	metadata[objects.FieldKeyCategory] = job.Category
	if msgConfig.Severity != emptyValue {
		metadata[objects.FieldKeySeverity] = msgConfig.Severity
		payload[objects.FieldKeySeverity] = msgConfig.Severity
	}

	// Create message
	message := types.Message{
		EventType: msgConfig.EventType,
		Source:    testMessageSourceScheduler,
		Timestamp: result.Timestamp,
		Payload:   payload,
		Metadata:  metadata,
	}

	// Route message
	err := RouteJobMessageAsync(ctx, h.asyncRouter, message)
	if err != nil {
		result.Error = err
		result.Success = false
		TestIOLog(h.logger).Warn(LogEventTestIORouteFailed).
			WithFields(append(append(jobLogFieldsWithErr(job, err), logging.Int("message_index", index)), logging.String("event_type", msgConfig.EventType))...).
			Log()
	} else {
		result.Success = true
		TestIOLog(h.logger).Info(LogEventTestIOMessageRouted).
			JobID(job.ID).
			MessageIndex(index).
			EventType(msgConfig.EventType).
			Log()
	}

	return result
}

// countSuccess counts successful test results
func countSuccess(results []TestIOResult) int {
	count := 0
	for _, result := range results {
		if result.Success {
			count++
		}
	}
	return count
}

// countFailures counts failed test results
func countFailures(results []TestIOResult) int {
	count := 0
	for _, result := range results {
		if !result.Success {
			count++
		}
	}
	return count
}

// createLogLevelTestMessages creates test messages for each log level
func (h *TestIOHandler) createLogLevelTestMessages(job *ScheduledJob) []TestMessageConfig {
	logLevels := []string{logLevelDefault, logLevelVerbose, string(pkgctx.ProfileDebug)}
	messages := make([]TestMessageConfig, 0, len(logLevels))

	for _, level := range logLevels {
		// Log at appropriate level to demonstrate log level behavior
		switch level {
		case logLevelDefault:
			TestIOLog(h.logger).Info(LogEventTestIOLogLevelDefaultDemo).
				JobID(job.ID).
				LogLevel(level).
				Log()
		case logLevelVerbose:
			TestIOLog(h.logger).Info(LogEventTestIOLogLevelVerboseDemo).
				JobID(job.ID).
				LogLevel(level).
				String("additional_context", "verbose level includes extra context").
				String("test_detail", "This message demonstrates verbose logging").
				Log()
		case string(pkgctx.ProfileDebug):
			TestIOLog(h.logger).Info(LogEventTestIOLogLevelDebugDemo).
				JobID(job.ID).
				LogLevel(level).
				String("debug_context", "debug level includes maximum detail").
				String("environment", "test").
				String("test_detail", "This message demonstrates debug logging").
				String("timestamp", zqktime.NowRFC3339UTC()).
				String("job_type", job.JobType).
				String("category", job.Category).
				Log()
		}

		messages = append(messages, TestMessageConfig{
			EventType: testLogLevelEventType,
			Severity:  messageSeverityLow,
			Payload: map[string]any{
				"test":                   true,
				objects.FieldKeyLogLevel: level,
				"job_id":                 job.ID,
				"timestamp":              zqktime.NowRFC3339UTC(),
				"message":                fmt.Sprintf("Test message for %s log level", level),
				objects.FieldKeySource:   testMessageSourceScheduler,
				objects.FieldKeyTestType: "log_level_verification",
			},
			Metadata: map[string]string{
				objects.FieldKeyLogLevel: level,
				objects.FieldKeyTestType: "log_level",
			},
		})
	}

	return messages
}
