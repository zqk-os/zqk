package scheduler

import (
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/scheduler/transceiver"
)

// Async settle delays after TestIOHandler.Execute (Phase E, CONSTANTS_AND_DRY_INVENTORY_PLAN).
const (
	testIOAsyncSettleAfterExecute = 200 * time.Millisecond
	testIOAsyncSettleAfterCustom  = 300 * time.Millisecond
)

func TestTestIOHandler_Execute(t *testing.T) {
	t.Parallel()
	_, storage := newSchedulerTestStorage(t)

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// Create router and async router
	router := transceiver.NewRouterWithDefaults(logger)
	asyncRouter := transceiver.NewAsyncRouter(pkgctx.NewSystemContext(), router, 5, 50, logger)

	// Start async router
	ctx := pkgctx.NewSystemContext()
	if err := asyncRouter.Start(ctx); err != nil {
		t.Fatalf("failed to start async router: %v", err)
	}
	defer func() { _ = asyncRouter.Stop() }() //nolint:errcheck // Test cleanup - error handling not critical

	// Create handler
	handler := NewTestIOHandler(storage, logger, asyncRouter)

	// Create test job with default config (tests all log levels)
	job := &ScheduledJob{
		ID:       "SCH-TEST-IO",
		JobType:  JobTypeTestIO,
		Category: "monitoring",
		Title:    "Test I/O Job",
		LogLevel: "debug", // Use debug to see all details
		EnvironmentVariables: map[string]string{
			EnvKeyTestIOTestLogLevels: "true",
			EnvKeyTestIOMessageDelay:  "50ms",
			EnvKeyTestIOFailOnError:   "false",
		},
	}

	// Execute handler
	err := handler.Execute(ctx, job)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Give async router time to process messages
	time.Sleep(testIOAsyncSettleAfterExecute)
}

func TestTestIOHandler_Execute_WithCustomMessages(t *testing.T) {
	t.Parallel()
	_, storage := newSchedulerTestStorage(t)

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// Create router and async router
	router := transceiver.NewRouterWithDefaults(logger)
	asyncRouter := transceiver.NewAsyncRouter(pkgctx.NewSystemContext(), router, 5, 50, logger)

	// Start async router
	ctx := pkgctx.NewSystemContext()
	if err := asyncRouter.Start(ctx); err != nil {
		t.Fatalf("failed to start async router: %v", err)
	}
	defer func() { _ = asyncRouter.Stop() }() //nolint:errcheck // Test cleanup - error handling not critical

	// Create handler
	handler := NewTestIOHandler(storage, logger, asyncRouter)

	// Create test job with custom messages
	job := &ScheduledJob{
		ID:       "SCH-TEST-IO-CUSTOM",
		JobType:  JobTypeTestIO,
		Category: "monitoring",
		Title:    "Test I/O Job - Custom Messages",
		LogLevel: "verbose",
		EnvironmentVariables: map[string]string{
			EnvKeyTestIOTestLogLevels: "false", // Disable log level testing
			EnvKeyTestIOTestMessages: `[
				{
					"event_type": "test_custom_message",
					"severity": "low",
					"payload": {
						"custom_field": "test_value"
					}
				},
				{
					"event_type": "test_custom_alert",
					"severity": "high",
					"payload": {
						"alert_type": "test"
					}
				}
			]`,
			EnvKeyTestIOMessageDelay: "100ms",
		},
	}

	// Execute handler
	err := handler.Execute(ctx, job)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Give async router time to process messages
	time.Sleep(testIOAsyncSettleAfterCustom)
}

func TestTestIOHandler_Execute_NoAsyncRouter(t *testing.T) {
	t.Parallel()
	_, storage := newSchedulerTestStorage(t)

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// Create handler WITHOUT async router (should fail)
	handler := NewTestIOHandler(storage, logger, nil)

	job := &ScheduledJob{
		ID:       "SCH-TEST-IO",
		JobType:  JobTypeTestIO,
		Category: "monitoring",
		Title:    "Test I/O Job",
	}

	// Execute handler - should fail
	ctx := pkgctx.NewSystemContext()
	err := handler.Execute(ctx, job)
	if err == nil {
		t.Error("Expected error when async router is nil, got nil")
	}
}

func TestTestIOHandler_LogLevels(t *testing.T) {
	t.Parallel()
	_, storage := newSchedulerTestStorage(t)

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// Create router and async router
	router := transceiver.NewRouterWithDefaults(logger)
	asyncRouter := transceiver.NewAsyncRouter(pkgctx.NewSystemContext(), router, 5, 50, logger)

	// Start async router
	ctx := pkgctx.NewSystemContext()
	if err := asyncRouter.Start(ctx); err != nil {
		t.Fatalf("failed to start async router: %v", err)
	}
	defer func() { _ = asyncRouter.Stop() }() //nolint:errcheck // Test cleanup - error handling not critical

	handler := NewTestIOHandler(storage, logger, asyncRouter)

	// Test each log level
	logLevels := []string{"default", "verbose", "debug"}

	for _, level := range logLevels {
		t.Run(level, func(t *testing.T) {
			job := &ScheduledJob{
				ID:       "SCH-TEST-IO-" + level,
				JobType:  JobTypeTestIO,
				Category: "monitoring",
				Title:    "Test I/O Job - " + level,
				LogLevel: level,
				EnvironmentVariables: map[string]string{
					EnvKeyTestIOTestLogLevels: "true",
					EnvKeyTestIOMessageDelay:  "50ms",
				},
			}

			err := handler.Execute(ctx, job)
			if err != nil {
				t.Fatalf("Execute failed for log level %s: %v", level, err)
			}

			// Give async router time to process
			time.Sleep(testIOAsyncSettleAfterExecute)
		})
	}
}
