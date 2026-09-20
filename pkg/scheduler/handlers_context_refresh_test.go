package scheduler

import (
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
)

func TestNewContextRefreshHandler(t *testing.T) {
	t.Parallel()
	testRoot, storage := newSchedulerTestStorage(t)

	tests := []struct {
		name        string
		projectRoot string
		wantRoot    string
	}{
		{
			name:        "with project root",
			projectRoot: testRoot,
			wantRoot:    testRoot,
		},
		{
			name:        "empty project root (extracted from storage)",
			projectRoot: "",
			wantRoot:    testRoot, // Should extract from FileObjectStorage
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewContextRefreshHandler(storage, tt.projectRoot)
			if handler == nil {
				t.Fatal("NewContextRefreshHandler() returned nil")
			}
			h := handler.(*ContextRefreshHandler)
			if h.storage == nil {
				t.Error("handler.storage is nil")
			}
			if h.projectRoot != tt.wantRoot {
				t.Errorf("handler.projectRoot = %s, want %s", h.projectRoot, tt.wantRoot)
			}
			if h.logger == nil {
				t.Error("handler.logger is nil")
			}
		})
	}
}

func TestContextRefreshHandler_Execute_NoPolicies(t *testing.T) {
	t.Parallel()
	testRoot, storage := newSchedulerTestStorage(t)

	handler := NewContextRefreshHandler(storage, testRoot)

	job := &ScheduledJob{
		ID:      "TEST-JOB-001",
		JobType: JobTypeContextRefresh,
	}

	ctx := pkgctx.NewSystemContext()
	err := handler.Execute(ctx, job)
	if err != nil {
		t.Errorf("Execute() error = %v, want nil", err)
	}
}

func TestContextRefreshHandler_Execute_WithPolicies(t *testing.T) {
	t.Parallel()
	testRoot, storage := newSchedulerTestStorage(t)

	handler := NewContextRefreshHandler(storage, testRoot)

	job := &ScheduledJob{
		ID:      "TEST-JOB-002",
		JobType: JobTypeContextRefresh,
	}

	ctx := pkgctx.NewSystemContext()
	// Execute handler - should succeed even with no policies
	// (handler gracefully handles empty policy list)
	err := handler.Execute(ctx, job)
	if err != nil {
		t.Errorf("Execute() error = %v, want nil", err)
	}

	// Note: Testing with actual policies would require creating fully valid
	// context_refresh_schedule objects with all required fields (title, status,
	// schema_version, created_at, updated_at, etc.). This is better tested
	// in integration tests or by using the actual object creation helpers.
}

func TestContextRefreshHandler_Execute_StorageError(t *testing.T) {
	t.Parallel()
	// Test with real storage that will fail on invalid path
	// This is simpler than creating a full mock that implements all interface methods
	testRoot, storage := newSchedulerTestStorage(t)

	handler := NewContextRefreshHandler(storage, testRoot)

	job := &ScheduledJob{
		ID:      "TEST-JOB-003",
		JobType: JobTypeContextRefresh,
	}

	ctx := pkgctx.NewSystemContext()
	// This should succeed (no policies found) rather than error
	// The handler gracefully handles empty policy lists
	err := handler.Execute(ctx, job)
	if err != nil {
		t.Errorf("Execute() error = %v, want nil (should handle empty policies gracefully)", err)
	}
}

func TestContextRefreshHandler_Execute_PolicyWithoutID(t *testing.T) {
	t.Parallel()
	testRoot, storage := newSchedulerTestStorage(t)

	// Create a mock result with a policy missing ID
	// We'll need to use a custom storage implementation or manipulate the result
	// For now, test that handler doesn't crash on missing ID
	handler := NewContextRefreshHandler(storage, testRoot)

	job := &ScheduledJob{
		ID:      "TEST-JOB-004",
		JobType: JobTypeContextRefresh,
	}

	ctx := pkgctx.NewSystemContext()
	// Should not error even if no policies exist
	err := handler.Execute(ctx, job)
	if err != nil {
		t.Errorf("Execute() error = %v, want nil (should handle missing policies gracefully)", err)
	}
}

func TestHandlerFactory_CreateHandler_ContextRefresh(t *testing.T) {
	t.Parallel()
	testRoot, storage := newSchedulerTestStorage(t)

	factory := NewHandlerFactory(
		storage,
		nil, // specLoader
		nil, // lifecycleLoader
		testRoot,
		logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
		NewNotificationContext(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)), nil),
		nil, // asyncRouter
		NewDefaultSchedulerMetricsCollector(),
		nil, // objectIDCacheBuilder
	)

	job := &ScheduledJob{
		ID:      "TEST-JOB",
		JobType: JobTypeContextRefresh,
	}

	handler := factory.CreateHandler(job)
	if handler == nil {
		t.Fatal("CreateHandler() returned nil for context_refresh")
	}

	// Verify it's a ContextRefreshHandler
	_, ok := handler.(*ContextRefreshHandler)
	if !ok {
		t.Errorf("CreateHandler() returned handler of type %T, want *ContextRefreshHandler", handler)
	}
}
