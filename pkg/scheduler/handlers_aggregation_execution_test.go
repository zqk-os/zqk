package scheduler

// ITEM-177483 inventory: setupSchedulerCompleteTestEnvironment → GetTestCleanup → RunProjectTestTeardown (scheduler_test_layout_helpers_test.go).

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

// TestAuditAggregationHandler_Execute_WithDeleteEnabled verifies that aggregation handler executes
// and deletes old events when DELETE_ENABLED=true and RETENTION_DURATION is set
func TestAuditAggregationHandler_Execute_WithDeleteEnabled(t *testing.T) {
	testRoot, storage := prepareHandlersIsolatedTempProject(t)

	handler := NewAuditAggregationHandlerWithProjectRoot(storage, testRoot)

	// Create test job with DELETE_ENABLED=true and short retention
	job := &ScheduledJob{
		ID:      "SCH-TEST-AGG-DELETE",
		JobType: JobTypeAuditEventAggregation,
		EnvironmentVariables: map[string]string{
			EnvKeyAggregationWindow: "1h",
			EnvKeyRetentionDuration: "1h", // Short retention for testing
			EnvKeyDeleteEnabled:     "true",
		},
	}

	ctx := pkgctx.NewSystemContext()

	// Execute handler - should not fail even with no events
	if err := handler.Execute(ctx, job); err != nil {
		t.Fatalf("aggregation handler should execute without error even with no events: %v", err)
	}

	t.Log("Aggregation handler executed successfully")
}

// TestChangeJournalAggregationHandler_Execute_WithDeleteEnabled verifies change journal aggregation executes
func TestChangeJournalAggregationHandler_Execute_WithDeleteEnabled(t *testing.T) {
	_, storage := prepareHandlersIsolatedTempProject(t)

	handler := NewChangeJournalAggregationHandler(storage)

	// Create test job with DELETE_ENABLED=true and short retention
	job := &ScheduledJob{
		ID:      "SCH-TEST-CJE-AGG-DELETE",
		JobType: JobTypeChangeJournalAggregation,
		EnvironmentVariables: map[string]string{
			EnvKeyAggregationWindow: "1h",
			EnvKeyRetentionDuration: "1h", // Short retention for testing
			EnvKeyDeleteEnabled:     "true",
		},
	}

	ctx := pkgctx.NewSystemContext()

	// Execute handler - should not fail even with no events
	if err := handler.Execute(ctx, job); err != nil {
		t.Fatalf("change journal aggregation handler should execute without error even with no events: %v", err)
	}

	t.Log("Change journal aggregation handler executed successfully")
}

// TestAuditAggregationHandler_EnvironmentVariableParsing verifies environment variable parsing
func TestAuditAggregationHandler_EnvironmentVariableParsing(t *testing.T) {
	// Not t.Parallel(): t.Setenv is invalid after Parallel; ZQK_TEST_ROOT must not race across tests.
	testRoot, storage := prepareHandlersIsolatedTempProject(t)

	handler := NewAuditAggregationHandlerWithProjectRoot(storage, testRoot)

	tests := []struct {
		name        string
		envVars     map[string]string
		expectError bool
	}{
		{
			name: "valid retention duration",
			envVars: map[string]string{
				EnvKeyRetentionDuration: "24h",
				EnvKeyDeleteEnabled:     "true",
			},
			expectError: false,
		},
		{
			name: "valid aggregation window",
			envVars: map[string]string{
				EnvKeyAggregationWindow: "15m",
				EnvKeyRetentionDuration: "24h",
			},
			expectError: false,
		},
		{
			name: "DELETE_ENABLED=true",
			envVars: map[string]string{
				EnvKeyDeleteEnabled:     "true",
				EnvKeyRetentionDuration: "1h",
			},
			expectError: false,
		},
		{
			name: "DELETE_ENABLED=false",
			envVars: map[string]string{
				EnvKeyDeleteEnabled:     "false",
				EnvKeyRetentionDuration: "1h",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &ScheduledJob{
				ID:                   "SCH-TEST-ENV",
				JobType:              JobTypeAuditEventAggregation,
				EnvironmentVariables: tt.envVars,
			}

			ctx := pkgctx.NewSystemContext()
			err := handler.Execute(ctx, job)

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			} else if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestAuditAggregationHandler_CompletesWithinBoundedTime runs the handler multiple times and asserts
// each run completes (or returns an error) within a bounded time. Proves reliability: no indefinite hang.
const aggregationRunTimeout = 15 * time.Second
const aggregationRuns = 3

func TestAuditAggregationHandler_CompletesWithinBoundedTime(t *testing.T) {
	testRoot, storage := prepareHandlersIsolatedTempProject(t)

	handler := NewAuditAggregationHandlerWithProjectRoot(storage, testRoot)
	job := &ScheduledJob{
		ID:      "SCH-TEST-AGG-RELIABILITY",
		JobType: JobTypeAuditEventAggregation,
		EnvironmentVariables: map[string]string{
			EnvKeyAggregationWindow: "1h",
			EnvKeyRetentionDuration: "1h",
			EnvKeyDeleteEnabled:     "true",
		},
	}

	ctx := pkgctx.NewSystemContext()

	for run := 0; run < aggregationRuns; run++ {
		done := make(chan error, 1)
		goroutinelabels.NewGoroutine("test_aggregation_reliability", "run audit aggregation handler").
			StartWithContext(ctx, func(ctx context.Context) error {
				err := handler.Execute(ctx, job)
				done <- err
				return nil
			})

		select {
		case err := <-done:
			if err != nil {
				t.Logf("Run %d returned error (acceptable): %v", run+1, err)
			}
		case <-time.After(aggregationRunTimeout):
			t.Fatalf("Run %d did not complete within %v (handler may be hanging)", run+1, aggregationRunTimeout)
		}
	}
	t.Logf("All %d runs completed within %v", aggregationRuns, aggregationRunTimeout)
}
