package scheduler

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/scheduler/transceiver"
	"github.com/lanceman/zqk/pkg/scheduler/transceiver/types"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestInvokeJobCallbackCommandExecutesWithHealthyAsyncRouter(t *testing.T) {
	t.Parallel()

	logger := logging.GetLoggerFromProfile("test")
	router := transceiver.NewRouterWithDefaults(logger)
	asyncRouter := transceiver.NewAsyncRouter(pkgctx.NewSystemContext(), router, 5, 50, logger)
	if err := asyncRouter.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = asyncRouter.Stop() }()

	callbackPath := filepath.Join(t.TempDir(), "callback.jsonl")
	job := &ScheduledJob{
		ID:                   "SCH-COMMAND-CALLBACK",
		JobType:              JobTypeRunWrapper,
		Category:             "testing",
		CallbackOnCompletion: "tee -a " + callbackPath,
	}
	InvokeJobCallback(context.Background(), logger, asyncRouter, job, jobCallbackEventCompletion, map[string]any{
		"outcome": "success",
	})

	content, err := fileutil.ReadFile(callbackPath)
	if err != nil {
		t.Fatalf("command callback evidence missing: %v", err)
	}
	if len(content) == 0 {
		t.Fatal("command callback evidence is empty")
	}
}

// A callback authored as a shell one-liner must run through a shell. Splitting it
// on whitespace made tee treat every remaining word (&&, --agent-id, message
// words) as an output file and litter the working directory.
func TestInvokeJobCallbackCommandRunsShellOneLinerWithoutLitteringWorkingDir(t *testing.T) {
	workDir := t.TempDir()
	t.Chdir(workDir)

	logger := logging.GetLoggerFromProfile("test")
	callbackPath := filepath.Join(workDir, "callback.jsonl")
	job := &ScheduledJob{
		ID:       "SCH-SHELL-CALLBACK",
		JobType:  JobTypeRunWrapper,
		Category: "testing",
		CallbackOnCompletion: "tee -a " + callbackPath + " >/dev/null && " +
			`printf '%s' "PLAN-CALLBACK done: emit status back to cursor-composer." > ` +
			filepath.Join(workDir, "steer.txt"),
	}
	InvokeJobCallback(context.Background(), logger, nil, job, jobCallbackEventCompletion, map[string]any{
		"outcome": "success",
	})

	content, err := fileutil.ReadFile(callbackPath)
	if err != nil {
		t.Fatalf("command callback evidence missing: %v", err)
	}
	if len(content) == 0 {
		t.Fatal("command callback evidence is empty")
	}
	if _, err := fileutil.Stat(filepath.Join(workDir, "steer.txt")); err != nil {
		t.Fatalf("chained callback command did not run: %v", err)
	}

	entries, err := fileutil.ReadDir(workDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		switch entry.Name() {
		case "callback.jsonl", "steer.txt":
		default:
			t.Errorf("callback littered the working directory with %q", entry.Name())
		}
	}
}

// capturedMessage holds a captured message for testing
//
//nolint:unused // Test helper - may be used in future tests
type capturedMessage struct {
	message types.Message
	mu      sync.Mutex
}

//nolint:unused // Test helper - may be used in future tests
//nolint:gocritic // hugeParam: types.Message is an interface type, must be passed by value
func (cm *capturedMessage) set(m types.Message) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.message = m
}

//nolint:unused // Test helper - may be used in future tests
func (cm *capturedMessage) get() types.Message {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	return cm.message
}

// TestRunWrapperHandler_CallbackRouting tests that callbacks are routed through transceiver
func TestRunWrapperHandler_CallbackRouting(t *testing.T) {
	t.Parallel()
	_, storage := newSchedulerTestStorage(t)

	logger := logging.GetLoggerFromProfile("test")

	// Create router and async router (use real router, test via RouteSync)
	router := transceiver.NewRouterWithDefaults(logger)
	asyncRouter := transceiver.NewAsyncRouter(pkgctx.NewSystemContext(), router, 5, 50, logger)

	// Create handler with async router
	handler := NewRunWrapperHandler(storage, logger, nil, asyncRouter)
	t.Cleanup(func() { handler.StopNotificationContext() })

	job := &ScheduledJob{
		ID:                   "SCH-TEST-CALLBACK",
		JobType:              JobTypeRunWrapper,
		Category:             "testing",
		CallbackOnCompletion: "https://example.com/webhook",
		CallbackType:         "webhook",
	}

	ctx := pkgctx.NewSystemContext()

	// Test completion callback routing
	payload := map[string]any{
		"success":  true,
		"duration": 1.5,
	}

	// Execute callback - it will route through async router
	// Since router isn't started, RouteAsync will fail and fall back to direct execution
	// So we test CreateCallbackMessage directly and verify the routing path via RouteSync
	handler.(*RunWrapperHandler).executeCallback(ctx, job, "completion", payload)

	// Test that CreateCallbackMessage creates correct message structure
	callbackMsg := CreateCallbackMessage(job, "completion", payload)
	if callbackMsg.EventType != "scheduler_job_callback_completion" {
		t.Errorf("Expected event type 'scheduler_job_callback_completion', got '%s'", callbackMsg.EventType)
	}

	if callbackMsg.Source != "scheduler" {
		t.Errorf("Expected source 'scheduler', got '%s'", callbackMsg.Source)
	}

	// Verify payload contains callback information
	if callbackMsg.Payload["job_id"] != job.ID {
		t.Errorf("Expected job_id '%s', got '%v'", job.ID, callbackMsg.Payload["job_id"])
	}

	if callbackMsg.Payload[objects.FieldKeyCallbackType] != "completion" {
		t.Errorf("Expected callback_type 'completion', got '%v'", callbackMsg.Payload[objects.FieldKeyCallbackType])
	}

	if callbackMsg.Payload["callback_url"] != job.CallbackOnCompletion {
		t.Errorf("Expected callback_url '%s', got '%v'", job.CallbackOnCompletion, callbackMsg.Payload["callback_url"])
	}

	if callbackMsg.Payload["callback_mechanism"] != "webhook" {
		t.Errorf("Expected callback_mechanism 'webhook', got '%v'", callbackMsg.Payload["callback_mechanism"])
	}

	// Verify original payload fields are preserved
	if callbackMsg.Payload["success"] != true {
		t.Errorf("Expected success=true, got %v", callbackMsg.Payload["success"])
	}

	if callbackMsg.Payload["duration"] != 1.5 {
		t.Errorf("Expected duration=1.5, got %v", callbackMsg.Payload["duration"])
	}
}

// TestRunWrapperHandler_CallbackRouting_ErrorCallback tests error callback routing
func TestRunWrapperHandler_CallbackRouting_ErrorCallback(t *testing.T) {
	t.Parallel()
	_, storage := newSchedulerTestStorage(t)

	logger := logging.GetLoggerFromProfile("test")
	router := transceiver.NewRouterWithDefaults(logger)
	asyncRouter := transceiver.NewAsyncRouter(pkgctx.NewSystemContext(), router, 5, 50, logger)
	handler := NewRunWrapperHandler(storage, logger, nil, asyncRouter)
	t.Cleanup(func() { handler.StopNotificationContext() })

	job := &ScheduledJob{
		ID:              "SCH-TEST-ERROR",
		JobType:         JobTypeRunWrapper,
		Category:        "testing",
		CallbackOnError: "https://example.com/error-webhook",
		CallbackType:    "webhook",
	}

	ctx := pkgctx.NewSystemContext()
	payload := map[string]any{
		"error":    "test error",
		"attempts": 3,
	}

	// Test error callback message creation
	callbackMsg := CreateCallbackMessage(job, "error", payload)
	if callbackMsg.EventType != "scheduler_job_callback_error" {
		t.Errorf("Expected event type 'scheduler_job_callback_error', got '%s'", callbackMsg.EventType)
	}

	if callbackMsg.Payload[objects.FieldKeyCallbackType] != "error" {
		t.Errorf("Expected callback_type 'error', got '%v'", callbackMsg.Payload[objects.FieldKeyCallbackType])
	}

	// Execute callback (will fall back to direct execution since router isn't started)
	handler.(*RunWrapperHandler).executeCallback(ctx, job, "error", payload)
}

// TestRunWrapperHandler_CallbackRouting_NoRouter tests fallback to direct execution
func TestRunWrapperHandler_CallbackRouting_NoRouter(t *testing.T) {
	t.Parallel()
	_, storage := newSchedulerTestStorage(t)

	logger := logging.GetLoggerFromProfile("test")

	// Create handler without async router (nil)
	handler := NewRunWrapperHandler(storage, logger, nil, nil)
	t.Cleanup(func() { handler.StopNotificationContext() })

	job := &ScheduledJob{
		ID:                   "SCH-TEST-NO-ROUTER",
		JobType:              JobTypeRunWrapper,
		Category:             "testing",
		CallbackOnCompletion: "https://example.com/webhook",
		CallbackType:         "webhook",
	}

	ctx := pkgctx.NewSystemContext()
	payload := map[string]any{
		"success": true,
	}

	// Should not panic when router is nil (should fall back to direct execution)
	// We can't easily test the direct execution here without mocking HTTP,
	// but we can verify it doesn't panic
	handler.(*RunWrapperHandler).executeCallback(ctx, job, "completion", payload)
}

// TestRunWrapperHandler_CallbackRouting_RouteFailure tests fallback when routing fails
func TestRunWrapperHandler_CallbackRouting_RouteFailure(t *testing.T) {
	t.Parallel()
	_, storage := newSchedulerTestStorage(t)

	logger := logging.GetLoggerFromProfile("test")

	// Create async router but don't start it - RouteAsync will fail with "async router not started"
	router := transceiver.NewRouterWithDefaults(logger)
	asyncRouter := transceiver.NewAsyncRouter(pkgctx.NewSystemContext(), router, 5, 50, logger)

	handler := NewRunWrapperHandler(storage, logger, nil, asyncRouter)
	t.Cleanup(func() { handler.StopNotificationContext() })

	job := &ScheduledJob{
		ID:                   "SCH-TEST-ROUTE-FAIL",
		JobType:              JobTypeRunWrapper,
		Category:             "testing",
		CallbackOnCompletion: "https://example.com/webhook",
		CallbackType:         "webhook",
	}

	ctx := pkgctx.NewSystemContext()
	payload := map[string]any{
		"success": true,
	}

	// Should not panic when routing fails (should fall back to direct execution)
	// RouteAsync will fail because router isn't started, triggering fallback
	handler.(*RunWrapperHandler).executeCallback(ctx, job, "completion", payload)
}

// TestCreateCallbackMessage tests the CreateCallbackMessage function
func TestCreateCallbackMessage(t *testing.T) {
	t.Parallel()
	job := &ScheduledJob{
		ID:       "SCH-TEST",
		JobType:  JobTypeRunWrapper,
		Category: "testing",
		Title:    "Test Job",
	}

	payload := map[string]any{
		"success":  true,
		"duration": 1.5,
	}

	// Test completion callback message
	msg := CreateCallbackMessage(job, "completion", payload)
	if msg.EventType != "scheduler_job_callback_completion" {
		t.Errorf("Expected event type 'scheduler_job_callback_completion', got '%s'", msg.EventType)
	}

	if msg.Source != "scheduler" {
		t.Errorf("Expected source 'scheduler', got '%s'", msg.Source)
	}

	if msg.Payload["job_id"] != job.ID {
		t.Errorf("Expected job_id '%s', got '%v'", job.ID, msg.Payload["job_id"])
	}

	if msg.Payload[objects.FieldKeyCallbackType] != "completion" {
		t.Errorf("Expected callback_type 'completion', got '%v'", msg.Payload[objects.FieldKeyCallbackType])
	}

	// Test error callback message
	msg = CreateCallbackMessage(job, "error", payload)
	if msg.EventType != "scheduler_job_callback_error" {
		t.Errorf("Expected event type 'scheduler_job_callback_error', got '%s'", msg.EventType)
	}

	// Test status callback message
	msg = CreateCallbackMessage(job, "status", payload)
	if msg.EventType != "scheduler_job_callback_status" {
		t.Errorf("Expected event type 'scheduler_job_callback_status', got '%s'", msg.EventType)
	}

	// Test with nil payload
	msg = CreateCallbackMessage(job, "completion", nil)
	if msg.Payload == nil {
		t.Error("Expected payload to be initialized even if nil is passed")
	}

	if msg.Payload["job_id"] != job.ID {
		t.Errorf("Expected job_id to be set, got '%v'", msg.Payload["job_id"])
	}
}
