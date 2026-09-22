package coordination

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCLINotifierAdapter_AllMethods(t *testing.T) {
	ctx := context.Background()

	// Test nil helper (safe no-op)
	nilAdapter := NewCLINotifierAdapter(nil)
	if err := nilAdapter.EmitProgress(ctx, "op1", "task", 1, 10, "msg", nil, false); err != nil {
		t.Errorf("unexpected error on nil helper: %v", err)
	}
	if err := nilAdapter.EmitStatusChange(ctx, "op1", "task", "old", "new", "msg", nil); err != nil {
		t.Errorf("unexpected error on nil helper: %v", err)
	}
	if err := nilAdapter.EmitError(ctx, "op1", "task", errors.New("fail"), "msg", nil); err != nil {
		t.Errorf("unexpected error on nil helper: %v", err)
	}
	if err := nilAdapter.EmitCompletion(ctx, "op1", "task", time.Second, "msg", nil); err != nil {
		t.Errorf("unexpected error on nil helper: %v", err)
	}

	// Test with valid coordinator and helper
	c := NewCoordinator(CoordinatorConfig{})
	helper := NewProgressHelper(c, "/tmp", "op-test", "test-task", "human")
	adapter := NewCLINotifierAdapter(helper)

	if err := adapter.EmitProgress(ctx, "op-test", "test-task", 5, 10, "in progress", map[string]any{"foo": "bar"}, false); err != nil {
		t.Errorf("unexpected error emitting progress: %v", err)
	}
	if err := adapter.EmitStatusChange(ctx, "op-test", "test-task", "pending", "running", "started", map[string]any{"k": "v"}); err != nil {
		t.Errorf("unexpected error emitting status change: %v", err)
	}
	if err := adapter.EmitError(ctx, "op-test", "test-task", errors.New("sample err"), "failed", map[string]any{"k": "v"}); err != nil {
		t.Errorf("unexpected error emitting error: %v", err)
	}
	if err := adapter.EmitCompletion(ctx, "op-test", "test-task", 500*time.Millisecond, "done", map[string]any{"k": "v"}); err != nil {
		t.Errorf("unexpected error emitting completion: %v", err)
	}
}

func TestProgressHelper_ExtraCoverage(t *testing.T) {
	ctx := context.Background()

	// Nil coordinator checks
	nilHelper := NewProgressHelper(nil, "/tmp", "op-nil", "type-nil", "human")
	if err := nilHelper.EmitProgressSummary(ctx, "50%", 5, 10, "halfway", nil); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := nilHelper.EmitStatusChange(ctx, "old", "new", "status changed", nil); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := nilHelper.EmitError(ctx, errors.New("err"), "err msg", nil); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := nilHelper.EmitCompletion(ctx, time.Second, "completed", nil); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Valid coordinator checks
	c := NewCoordinator(CoordinatorConfig{})
	helper := NewProgressHelper(c, "/tmp", "op-prog", "type-prog", "human")

	// EmitProgressSummary
	if err := helper.EmitProgressSummary(ctx, "25%", 25, 100, "quarter", map[string]any{"phase": 1}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// EmitStatusChange: first time (should emit)
	if err := helper.EmitStatusChange(ctx, "init", "started", "msg 1", map[string]any{"p": 1}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// EmitStatusChange: duplicate transition (should dedup and return nil)
	if err := helper.EmitStatusChange(ctx, "init", "started", "msg 1 again", map[string]any{"p": 1}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// EmitStatusChange: heartbeat where old == new
	if err := helper.EmitStatusChange(ctx, "started", "started", "heartbeat", nil); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// EmitError with and without err/message
	if err := helper.EmitError(ctx, errors.New("boom"), "something broke", map[string]any{"detail": "err"}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := helper.EmitError(ctx, nil, "", nil); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// EmitCompletion with and without message
	if err := helper.EmitCompletion(ctx, 2*time.Second, "all done", map[string]any{"extra": true}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := helper.EmitCompletion(ctx, 0, "", nil); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestErrorHelper_AllMethods(t *testing.T) {
	ctx := context.Background()

	// Nil coordinator checks
	nilHelper := NewErrorHelper(nil, "/tmp", "op-err", "type-err", "human")
	if err := nilHelper.EmitError(ctx, errors.New("fail"), "msg", nil, "high"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := nilHelper.EmitWarning(ctx, "warn msg", nil); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Valid coordinator checks
	c := NewCoordinator(CoordinatorConfig{})
	helper := NewErrorHelper(c, "/tmp", "op-err", "type-err", "human")

	// EmitError with various parameters
	if err := helper.EmitError(ctx, errors.New("failed op"), "err message", map[string]any{"f": "v"}, "critical"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := helper.EmitError(ctx, nil, "", nil, ""); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// EmitWarning with and without message
	if err := helper.EmitWarning(ctx, "watch out", map[string]any{"f": "v"}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := helper.EmitWarning(ctx, "", nil); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// EmitRetryableError (retryCount < maxRetries and retryCount >= maxRetries)
	if err := helper.EmitRetryableError(ctx, errors.New("retryable"), "transient error", 1, 3, nil); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := helper.EmitRetryableError(ctx, errors.New("retryable final"), "permanent error", 3, 3, map[string]any{"k": 1}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// EmitTimeoutError
	if err := helper.EmitTimeoutError(ctx, 5*time.Second, "deadline exceeded", nil); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := helper.EmitTimeoutError(ctx, 100*time.Millisecond, "quick timeout", map[string]any{"timeout": true}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGlobalCoordinator(t *testing.T) {
	// Reset to ensure clean start
	ResetGlobalCoordinator()

	// GetCoordinator initializes default
	c1 := GetCoordinator()
	if c1 == nil {
		t.Fatalf("expected non-nil coordinator")
	}

	// Calling GetCoordinator again returns the same instance
	c2 := GetCoordinator()
	if c1 != c2 {
		t.Errorf("expected same coordinator instance")
	}

	// SetGlobalCoordinator replaces it
	custom := NewCoordinator(CoordinatorConfig{})
	SetGlobalCoordinator(custom)
	if GetCoordinator() != custom {
		t.Errorf("expected custom coordinator to be set")
	}

	// Reset clears it
	ResetGlobalCoordinator()
}

func TestDefaultRoutersAndEventContext(t *testing.T) {
	ctx := context.Background()

	// DefaultAuditRouter
	ar := &DefaultAuditRouter{}
	if err := ar.Emit(ctx, &EventContext{}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// DefaultMetricsRouter
	mr := &DefaultMetricsRouter{}
	if err := mr.Emit(ctx, &EventContext{}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// DefaultOperationalRouter
	opRouter := &DefaultOperationalRouter{}
	sub := &testSubscriber{id: "sub-1", active: true}
	evCtx := NewEventContext("op1", "type1", "complete")
	event := NewOperationalEvent(evCtx)
	if err := opRouter.Emit(ctx, event, []OperationalEventSubscriber{sub}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// EventContext WithLevel
	evCtxWithLevel := NewEventContext("op1", "type1", "complete").WithLevel("debug")
	if evCtxWithLevel.Level != "debug" {
		t.Errorf("expected debug level, got %s", evCtxWithLevel.Level)
	}
}

func TestStorageAuditRouter_InferHelpers(t *testing.T) {
	router := &StorageAuditRouter{}

	// inferEventType tests
	// orphan_cleanup
	if got := router.inferEventType("orphan_cleanup_job", OperationStatusError); got != "orphan_cleanup_error" {
		t.Errorf("expected orphan_cleanup_error, got %s", got)
	}
	if got := router.inferEventType("orphan_cleanup_job", "failure"); got != "orphan_cleanup_error" {
		t.Errorf("expected orphan_cleanup_error, got %s", got)
	}
	if got := router.inferEventType("orphan_cleanup_job", OperationStatusStart); got != "orphan_cleanup_start" {
		t.Errorf("expected orphan_cleanup_start, got %s", got)
	}
	if got := router.inferEventType("orphan_cleanup_worker_started", "running"); got != "orphan_cleanup_start" {
		t.Errorf("expected orphan_cleanup_start, got %s", got)
	}
	if got := router.inferEventType("orphan_cleanup_job", OperationStatusComplete); got != "orphan_cleanup_complete" {
		t.Errorf("expected orphan_cleanup_complete, got %s", got)
	}

	// Standard operation types
	if got := router.inferEventType("custom_op", OperationStatusError); got != "custom_op_error" {
		t.Errorf("expected custom_op_error, got %s", got)
	}
	if got := router.inferEventType("custom_op", OperationStatusComplete); got != "custom_op_complete" {
		t.Errorf("expected custom_op_complete, got %s", got)
	}
	if got := router.inferEventType("custom_op", OperationStatusStart); got != "custom_op_start" {
		t.Errorf("expected custom_op_start, got %s", got)
	}
	if got := router.inferEventType("custom_op", "custom_status"); got != "custom_op_custom_status" {
		t.Errorf("expected custom_op_custom_status, got %s", got)
	}

	// inferSeverity tests
	if got := router.inferSeverity(OperationStatusError); got != "high" {
		t.Errorf("expected high, got %s", got)
	}
	if got := router.inferSeverity(OperationStatusComplete); got != "medium" {
		t.Errorf("expected medium, got %s", got)
	}
	if got := router.inferSeverity("unknown"); got != "low" {
		t.Errorf("expected low, got %s", got)
	}
}

func TestNewCLINotifierWithCoordinator_Success(t *testing.T) {
	testRoot, fileStorage, cleanup := setupCoordinationCompleteTestEnvironment(t, nil)
	defer cleanup()

	notifier := NewCLINotifierWithCoordinator(false, false, testRoot, fileStorage, "op-cli-test", "testing", "human")
	if notifier == nil {
		t.Fatalf("expected non-nil OperationNotifier")
	}
}
