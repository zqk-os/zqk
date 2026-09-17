package utility

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/coordination"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/objects"
)

// emitCoordinatorEvent is a helper to emit events via coordinator (defined in scenario_builder.go)
// This allows scenario_builder_data_loader.go to use the same coordinator instance
// Coordinator is required - this will panic if coordinator is nil (should never happen in normal operation)
func (sb *ScenarioBuilder) emitCoordinatorEvent(ctx context.Context, operationType, status, message string, fields map[string]any) {
	if sb.coordinator == nil {
		// Coordinator is required - this indicates a programming error
		// In normal operation, coordinator is always set by createScenarioBuilder
		// TRACK: [Test utility missing prerequisite]
		panic("coordinator is required for ScenarioBuilder - all events must flow through coordinator")
	}

	// Build logging fields
	loggingFields := make([]coordination.LoggingField, 0, len(fields))
	for k, v := range fields {
		loggingFields = append(loggingFields, coordination.LoggingField{Key: k, Value: v})
	}
	loggingFields = append(loggingFields, coordination.LoggingField{Key: scenarioBuilderMetadataEvent, Value: message})

	// Build audit metadata for scenario builder operations
	auditMetadata := make(map[string]any)
	if operationType == ScenarioBuilderProfileName {
		// Include relevant metadata for audit trail
		if targetDir, ok := fields["target_dir"].(string); ok {
			auditMetadata[objects.FieldKeyTargetPath] = targetDir
		}
		if objectCount, ok := fields[objects.FieldKeyObjectCount].(int); ok {
			auditMetadata[objects.FieldKeyObjectCount] = objectCount
		} else if objectCountFloat, ok := fields[objects.FieldKeyObjectCount].(float64); ok {
			auditMetadata[objects.FieldKeyObjectCount] = int(objectCountFloat)
		}
		if kind, ok := fields[objects.FieldKeyKind].(string); ok {
			auditMetadata[objects.FieldKeyTargetKind] = kind
		}
		if count, ok := fields["count"].(int); ok {
			auditMetadata["count"] = count
		} else if countFloat, ok := fields["count"].(float64); ok {
			auditMetadata["count"] = int(countFloat)
		}
		auditMetadata[objects.FieldKeyOperation] = message
	}

	// Create event data
	eventData := &coordination.EventData{
		LoggingFields: loggingFields,
		AuditMetadata: auditMetadata, // Enable audit events for scenario builder operations
		MetricsData:   nil,           // Metrics can be added later
	}

	// Create event context
	operationID := fmt.Sprintf("scenario-builder-%d", time.Now().UnixNano())
	eventCtx := coordination.NewEventContext(operationID, operationType, status).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(true, true, false, true) // Logging, audit, and operational (for subscriber)

	// Emit via coordinator
	_ = sb.coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Best effort
}

// getLockMetricsRecorder gets the metrics recorder for lock operations
// Uses the factory pattern to get a self-registering, decoupled metrics recorder
func (sb *ScenarioBuilder) getLockMetricsRecorder() MetricRecorder {
	// For now, return no-op - can be extended to get from coordinator or factory
	// This allows the new API to be in place without requiring full implementation
	return GetNoOpMetricRecorder()
}

// withLock executes a function while holding a mutex lock, ensuring unlock via defer
func withLock(mu *sync.Mutex, fn func()) {
	mu.Lock()
	defer mu.Unlock()
	fn()
}

// withLockTimeout executes a function while holding a mutex lock with timeout support
// The lock is acquired in the current goroutine (required for proper unlock).
// The timeout applies to the operation execution, not lock acquisition.
// Returns an error if the operation times out or if ctx is cancelled.
// Records metrics using the new factory-based metrics API.
func (sb *ScenarioBuilder) withLockTimeout(ctx context.Context, mu *sync.Mutex, operationName string, fn func() error) error {
	recorder := sb.getLockMetricsRecorder()
	// Track lock acquisition time
	acquireStart := time.Now()

	// CRITICAL: Acquire lock in current goroutine (required for proper unlock)
	// sync.Mutex.Lock() doesn't support timeout, so we block here
	// The timeout applies to the operation execution, not lock acquisition
	mu.Lock()

	// Record wait time (time to acquire lock)
	waitTime := time.Since(acquireStart)
	if recorder.IsEnabled() && waitTime > 10*time.Millisecond {
		_ = recorder.Record("lock_wait", NewMetricBuilder("lock_wait").
			WithField("operation", operationName).
			WithField("wait_time_ms", waitTime.Milliseconds()).
			WithDuration(waitTime).
			WithTags(ScenarioBuilderProfileName, "lock", "wait"))
	}

	// Track lock hold time
	holdStart := time.Now()
	defer func() {
		holdTime := time.Since(holdStart)
		if recorder.IsEnabled() && holdTime > 100*time.Millisecond {
			_ = recorder.Record("lock_hold", NewMetricBuilder("lock_hold").
				WithField("operation", operationName).
				WithField("hold_time_ms", holdTime.Milliseconds()).
				WithDuration(holdTime).
				WithTags(ScenarioBuilderProfileName, "lock", "hold"))
		}
		mu.Unlock()
	}()

	// Execute function with timeout monitoring
	done := make(chan error, 1)
	goroutinelabels.NewGoroutine("refactor", "refactored").StartSimple(func() {
		func() {
			done <- fn()
		}()
	})

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return errfmt.Errorf("operation timed out: %w", ctx.Err())
	}
}

// withLockTimeoutDuration is a convenience wrapper that creates a context with timeout
// withLockTimeoutDuration executes a function while holding a mutex lock with duration-based timeout
// This is a convenience wrapper around withLockTimeout that uses a duration instead of context.
func (sb *ScenarioBuilder) withLockTimeoutDuration(mu *sync.Mutex, timeout time.Duration, operationName string, fn func() error) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout) // Background: request-or-shutdown derived
	defer cancel()
	return sb.withLockTimeout(ctx, mu, operationName, fn)
}

// handleLoadDataFile is defined in scenario_builder_data_loader_load.go

// BuildFromDataFile builds scenario from loaded data file objects
func (sb *ScenarioBuilder) BuildFromDataFile(ctx context.Context) error {
	return sb.BuildFromDataFileViaPipeline(ctx)
}

// prepareObjectFromDataFile, setFieldsDirectly, validateObjectBeforeCreation are defined in scenario_builder_data_loader_prepare.go
// resolveReferencesFromIDStream, verifyReferencedObjectsExistMemoized, referenceExistsMemoized are defined in scenario_builder_data_loader_references.go
// getObjectFilePath, updateObjectIDCacheSync, reconstructFilePath are defined in scenario_builder_data_loader_paths.go
// objectRefs, buildDependencyGraphAndSort, usesContentAddressableStorage are defined in scenario_builder_data_loader_dependency.go
