package transceiver

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/scheduler/transceiver/types"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestProfileLoader_FullExtractionAndInheritance verifies profile loading, type checking,
// spec extraction (including all default/fallback cases), and inheritance.
func TestProfileLoader_FullExtractionAndInheritance(t *testing.T) {
	tempDir := t.TempDir()

	validProfileYAML := `schema_version: "2.0.0"
kind: profile
type: transceiver_router
metadata:
  name: valid_router
  description: "Test router profile"
spec:
  enabled: true
  max_workers: 24
  queue_size: 350
  enqueue_timeout_ms: 125
  default_timeout_ms: 6000
  backoff_strategy: "linear"
  max_backoff_ms: 20000
  metrics_interval_ms: 500
  is_default: true
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(tempDir, "valid_router.yaml"), []byte(validProfileYAML), paths.FilePerm644))

	// Wrong profile type (cli_context instead of transceiver_router)
	wrongTypeYAML := `schema_version: "2.0.0"
kind: profile
type: cli_context
metadata:
  name: wrong_type_router
spec:
  something: 123
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(tempDir, "wrong_type_router.yaml"), []byte(wrongTypeYAML), paths.FilePerm644))

	loader := NewRouterProfileLoader(tempDir, nil)
	require.NotNil(t, loader)

	// 1. Load valid profile
	spec, err := loader.LoadProfile("valid_router")
	require.NoError(t, err)
	require.NotNil(t, spec)
	assert.True(t, spec.Enabled)
	assert.Equal(t, 24, spec.MaxWorkers)
	assert.Equal(t, 350, spec.QueueSize)
	assert.Equal(t, 125, spec.EnqueueTimeoutMs)
	assert.Equal(t, 6000, spec.DefaultTimeoutMs)
	assert.Equal(t, "linear", spec.BackoffStrategy)
	assert.Equal(t, 20000, spec.MaxBackoffMs)
	assert.Equal(t, 500, spec.MetricsIntervalMs)
	assert.True(t, spec.IsDefault)

	// 2. Load wrong type profile -> error
	_, errWrongType := loader.LoadProfile("wrong_type_router")
	require.Error(t, errWrongType)
	assert.Contains(t, errWrongType.Error(), "is not a transceiver_router profile")

	// 3. Load non-existent profile -> error
	_, errNotFound := loader.LoadProfile("missing_router_profile_xyz")
	require.Error(t, errNotFound)
	assert.Contains(t, errNotFound.Error(), "failed to load profile")

	// 4. Test extractRouterSpec with empty and boundary values (to hit all fallback branches)
	defaultSpec, errDefault := loader.extractRouterSpec(map[string]any{})
	require.NoError(t, errDefault)
	assert.True(t, defaultSpec.Enabled)
	assert.Equal(t, 10, defaultSpec.MaxWorkers)
	assert.Equal(t, 100, defaultSpec.QueueSize)
	assert.Equal(t, 100, defaultSpec.EnqueueTimeoutMs)
	assert.Equal(t, 5000, defaultSpec.DefaultTimeoutMs)
	assert.Equal(t, "exponential", defaultSpec.BackoffStrategy)
	assert.Equal(t, 30000, defaultSpec.MaxBackoffMs)
	assert.Equal(t, 0, defaultSpec.MetricsIntervalMs)
	assert.False(t, defaultSpec.IsDefault)

	// extractRouterSpec with negative / invalid values that trigger boundary fallbacks
	boundarySpec, errBoundary := loader.extractRouterSpec(map[string]any{
		objects.FieldKeyEnabled:   false,
		"max_workers":             -5,
		"queue_size":              0,
		"enqueue_timeout_ms":      -1,
		"default_timeout_ms":      0,
		"backoff_strategy":        "",
		"max_backoff_ms":          -100,
		"metrics_interval_ms":     250,
		objects.FieldKeyIsDefault: false,
	})
	require.NoError(t, errBoundary)
	assert.False(t, boundarySpec.Enabled)
	assert.Equal(t, 10, boundarySpec.MaxWorkers) // unchanged from default
	assert.Equal(t, 100, boundarySpec.QueueSize)
	assert.Equal(t, 100, boundarySpec.EnqueueTimeoutMs)
	assert.Equal(t, 5000, boundarySpec.DefaultTimeoutMs)
	assert.Equal(t, "exponential", boundarySpec.BackoffStrategy)
	assert.Equal(t, 30000, boundarySpec.MaxBackoffMs)
	assert.Equal(t, 250, boundarySpec.MetricsIntervalMs)
}

// TestProfileLoader_NewAsyncRouterFromProfile tests NewAsyncRouterFromProfile with both
// a missing profile (error path) and a newly provisioned profile on the filesystem.
func TestProfileLoader_NewAsyncRouterFromProfile(t *testing.T) {
	ctx := context.Background()
	router := NewRouter(logging.GetLoggerFromProfile("test"))

	// 1. Missing profile error path
	_, errMissing := NewAsyncRouterFromProfile(ctx, router, "non_existent_profile_spec", nil)
	require.Error(t, errMissing)
	assert.Contains(t, errMissing.Error(), "failed to load router profile")

	// 2. Discoverable profile path using paths.ProcessInternalProfileSpecsDir in a temporary root
	tempDir := t.TempDir()
	specsDir := filepath.Join(tempDir, paths.ProcessInternalProfileSpecsDir)
	require.NoError(t, fileutil.MkdirAll(specsDir, paths.DirPerm755))

	testProfilePath := filepath.Join(specsDir, "c33_test_router.yaml")
	profileYAML := `schema_version: "2.0.0"
kind: profile
type: transceiver_router
metadata:
  name: c33_test_router
spec:
  enabled: true
  max_workers: 4
  queue_size: 50
  enqueue_timeout_ms: 100
  default_timeout_ms: 2000
`
	errWrite := fileutil.WriteFile(testProfilePath, []byte(profileYAML), paths.FilePerm644)
	require.NoError(t, errWrite)

	t.Chdir(tempDir)
	paths.ResetCwdDiscovery()
	t.Cleanup(func() {
		paths.ResetCwdDiscovery()
	})

	asyncRouter, errProfile := NewAsyncRouterFromProfile(ctx, router, "c33_test_router", nil)
	require.NoError(t, errProfile)
	require.NotNil(t, asyncRouter)
	assert.Equal(t, 4, asyncRouter.maxWorkers)
	assert.Equal(t, 50, asyncRouter.GetQueueCapacity())
}

// TestMessageVerification_StartCleanup tests StartCleanup and background ticker loop.
func TestMessageVerification_StartCleanup(t *testing.T) {
	mv := NewMessageVerification(nil, 20*time.Millisecond)

	mv.RegisterMessage("cleanup-msg-1", "job_event", []string{"http://endpoint-a", "http://endpoint-b"})
	mv.RegisterMessage("cleanup-msg-2", "job_event", []string{"http://endpoint-c"})

	ctx, cancel := context.WithCancel(context.Background())
	mv.StartCleanup(ctx)

	// Wait past expiration
	time.Sleep(30 * time.Millisecond)
	mv.CleanupExpired()

	res1, found1 := mv.GetVerificationResult("cleanup-msg-1")
	assert.True(t, found1)
	assert.False(t, res1.Success)
	assert.Len(t, res1.FailedURLs, 2)

	// Cancel context to cleanly shut down StartCleanup goroutine
	cancel()
	time.Sleep(20 * time.Millisecond)
}

type blockAdapter struct {
	MockAdapter
	block chan struct{}
}

func (b *blockAdapter) Send(ctx context.Context, message types.Message, action types.Action) error {
	<-b.block
	return nil
}

// TestAsyncRouter_RouteAsyncWithTimeout_AllBranches covers all select branches in RouteAsyncWithTimeout.
func TestAsyncRouter_RouteAsyncWithTimeout_AllBranches(t *testing.T) {
	ctx := context.Background()
	router := NewRouter(nil)
	msg := types.Message{EventType: "test.timeout"}

	// Branch 1: Not started
	arNotStarted := NewAsyncRouter(ctx, router, 1, 10, nil)
	errNotStarted := arNotStarted.RouteAsyncWithTimeout(ctx, msg, 50*time.Millisecond)
	require.Error(t, errNotStarted)
	assert.Contains(t, errNotStarted.Error(), "async router not started")

	// Branch 2: Shutdown already initiated
	arShutdown := NewAsyncRouter(ctx, router, 1, 10, nil)
	require.NoError(t, arShutdown.Start(ctx))
	require.NoError(t, arShutdown.InitiateShutdown())

	errShutdown := arShutdown.RouteAsyncWithTimeout(ctx, msg, 50*time.Millisecond)
	require.Error(t, errShutdown)
	assert.Contains(t, errShutdown.Error(), "async router is shutting down")
	_ = arShutdown.Stop()

	// Branch 3: Queue full and timeout triggered (metrics recorded)
	blockChan := make(chan struct{})
	blockAd := &blockAdapter{
		MockAdapter: MockAdapter{name: "block_proto"},
		block:       blockChan,
	}
	blockRouter := NewRouter(nil)
	require.NoError(t, blockRouter.RegisterAdapter(blockAd))
	blockRouter.LoadRules([]RoutingRule{
		{
			Name:    "block_rule",
			Enabled: true,
			Match:   MessageMatcher{EventType: "test.timeout"},
			Actions: []Action{{Protocol: "block_proto", Endpoint: "wait"}},
		},
	})

	arFull := NewAsyncRouter(ctx, blockRouter, 1, 1, nil) // 1 worker, capacity 1
	require.NoError(t, arFull.Start(ctx))

	// First message is picked up by worker and blocks on blockChan
	errQ1 := arFull.RouteAsyncWithTimeout(ctx, msg, 100*time.Millisecond)
	require.NoError(t, errQ1)
	time.Sleep(20 * time.Millisecond) // Ensure worker has started processing Q1

	// Second message fills the 1-slot queue
	errQ2 := arFull.RouteAsyncWithTimeout(ctx, msg, 100*time.Millisecond)
	require.NoError(t, errQ2)

	// Third message times out because queue is full and worker is busy
	errQ3 := arFull.RouteAsyncWithTimeout(ctx, msg, 10*time.Millisecond)
	require.Error(t, errQ3)
	assert.Contains(t, errQ3.Error(), "queue full or timeout")

	// Unblock worker and shut down
	close(blockChan)
	_ = arFull.Stop()

	// Branch 4: Shutdown occurs while waiting on queue
	blockChan2 := make(chan struct{})
	blockAd2 := &blockAdapter{
		MockAdapter: MockAdapter{name: "block_proto2"},
		block:       blockChan2,
	}
	blockRouter2 := NewRouter(nil)
	require.NoError(t, blockRouter2.RegisterAdapter(blockAd2))
	blockRouter2.LoadRules([]RoutingRule{
		{
			Name:    "block_rule2",
			Enabled: true,
			Match:   MessageMatcher{EventType: "test.timeout"},
			Actions: []Action{{Protocol: "block_proto2", Endpoint: "wait"}},
		},
	})

	arWaitShutdown := NewAsyncRouter(ctx, blockRouter2, 1, 1, nil)
	require.NoError(t, arWaitShutdown.Start(ctx))
	require.NoError(t, arWaitShutdown.RouteAsyncWithTimeout(ctx, msg, 100*time.Millisecond))
	time.Sleep(20 * time.Millisecond)                                                        // Worker busy with first message
	require.NoError(t, arWaitShutdown.RouteAsyncWithTimeout(ctx, msg, 100*time.Millisecond)) // Queue filled

	// In background, initiate shutdown shortly
	goroutinelabels.NewGoroutine("test_async_router_shutdown", "initiate shutdown during wait").StartSimple(func() {
		time.Sleep(15 * time.Millisecond)
		_ = arWaitShutdown.InitiateShutdown()
	})

	errWaitSD := arWaitShutdown.RouteAsyncWithTimeout(ctx, msg, 500*time.Millisecond)
	require.Error(t, errWaitSD)
	assert.Contains(t, errWaitSD.Error(), "async router is shutting down")
	close(blockChan2)
	_ = arWaitShutdown.Stop()
}

// TestAsyncRouter_Worker_FailureAndEvents verifies worker error logging and event callbacks.
func TestAsyncRouter_Worker_FailureAndEvents(t *testing.T) {
	ctx := context.Background()
	router := NewRouter(nil)

	failAdapter := &MockAdapter{
		name:      "failing_adapter",
		sendError: errors.New("underlying network unreachable"),
	}
	require.NoError(t, router.RegisterAdapter(failAdapter))

	router.LoadRules([]RoutingRule{
		{
			Name:    "failing_rule",
			Enabled: true,
			Match:   MessageMatcher{EventType: "job.fail"},
			Actions: []Action{{Protocol: "failing_adapter", Endpoint: "nowhere"}},
		},
	})

	var callbackInvoked atomic.Bool
	SetAsyncRouterEventCallback(func(
		ctx context.Context,
		projectRoot string,
		storageProvider any,
		workerID string,
		eventType string,
		status string,
		workerCount int,
		processedCount int,
		failedCount int,
		duration time.Duration,
	) {
		callbackInvoked.Store(true)
		assert.Equal(t, "/tmp/mock-project-root", projectRoot)
	})
	t.Cleanup(func() {
		SetAsyncRouterEventCallback(nil)
	})

	ar := NewAsyncRouter(ctx, router, 2, 10, nil)
	ar.SetProjectRoot("/tmp/mock-project-root")
	ar.SetStorageProvider("mock-provider")
	require.NoError(t, ar.Start(ctx))

	// Enqueue message that triggers failing route
	errRoute := ar.RouteAsync(ctx, types.Message{EventType: "job.fail"})
	require.NoError(t, errRoute)

	// Wait for worker to process and fail
	time.Sleep(50 * time.Millisecond)

	// Stop router (which shuts down workers and triggers defer callback)
	require.NoError(t, ar.Stop())
	assert.True(t, callbackInvoked.Load())

	// Stop on already stopped router is idempotent
	assert.NoError(t, ar.Stop())

	// Stop on unstarted router is no-op
	unstarted := NewAsyncRouter(ctx, router, 1, 5, nil)
	assert.NoError(t, unstarted.Stop())
}

// TestAsyncRouter_Drain_TimeoutAndCancellation tests context cancellation during Drain.
func TestAsyncRouter_Drain_TimeoutAndCancellation(t *testing.T) {
	ctx := context.Background()
	router := NewRouter(nil)
	ar := NewAsyncRouter(ctx, router, 1, 10, nil)
	require.NoError(t, ar.Start(ctx))

	// Cancelled context during drain
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()

	errDrain := ar.Drain(cancelCtx)
	require.Error(t, errDrain)
	assert.Equal(t, context.Canceled, errDrain)

	_ = ar.Stop()
}

// TestRouter_ExecuteWithRetry_ExhaustionAndCancellation tests retry exhaustion and context cancellation.
func TestRouter_ExecuteWithRetry_ExhaustionAndCancellation(t *testing.T) {
	router := NewRouter(nil)

	// 1. Retry exhaustion
	failAdapter := &MockAdapter{
		name:      "flaky",
		sendError: errors.New("disk write failure"),
	}
	require.NoError(t, router.RegisterAdapter(failAdapter))

	actionRetry := types.Action{
		Protocol: "flaky",
		Endpoint: "loc",
		Retry: &types.RetryConfig{
			MaxAttempts:  2,
			InitialDelay: time.Millisecond,
			MaxDelay:     5 * time.Millisecond,
			Backoff:      "linear",
		},
	}
	msg := types.Message{EventType: "ev"}

	errExhausted := router.executeWithRetry(context.Background(), failAdapter, msg, actionRetry, "rule1")
	require.Error(t, errExhausted)
	assert.Contains(t, errExhausted.Error(), "action failed after 2 attempts")

	// 2. Context cancellation during retry wait
	cancelCtx, cancel := context.WithCancel(context.Background())
	actionSlowRetry := types.Action{
		Protocol: "flaky",
		Endpoint: "loc",
		Retry: &types.RetryConfig{
			MaxAttempts:  5,
			InitialDelay: 500 * time.Millisecond,
			MaxDelay:     time.Second,
			Backoff:      "exponential",
		},
	}
	goroutinelabels.NewGoroutine("test_retry_cancel", "cancel context during retry backoff").StartSimple(func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	})

	errCancel := router.executeWithRetry(cancelCtx, failAdapter, msg, actionSlowRetry, "rule2")
	require.Error(t, errCancel)
	assert.Equal(t, context.Canceled, errCancel)

	// 3. Retry with MaxAttempts <= 0 defaults to 1 attempt
	actionZeroAttempts := types.Action{
		Protocol: "flaky",
		Endpoint: "loc",
		Retry: &types.RetryConfig{
			MaxAttempts: 0,
		},
	}
	errZero := router.executeWithRetry(context.Background(), failAdapter, msg, actionZeroAttempts, "rule3")
	require.Error(t, errZero)
	assert.Contains(t, errZero.Error(), "action failed after 1 attempts")
}

// TestRouter_CalculateBackoff_EdgeCases tests all backoff branches and caps.
func TestRouter_CalculateBackoff_EdgeCases(t *testing.T) {
	router := NewRouter(nil)

	// Exponential exceeding maxDelay
	d1 := router.calculateBackoff(20*time.Second, &types.RetryConfig{
		Backoff:  "exponential",
		MaxDelay: 30 * time.Second,
	})
	assert.Equal(t, 30*time.Second, d1)

	// Linear exceeding maxDelay
	d2 := router.calculateBackoff(25*time.Second, &types.RetryConfig{
		Backoff:      "linear",
		InitialDelay: 10 * time.Second,
		MaxDelay:     30 * time.Second,
	})
	assert.Equal(t, 30*time.Second, d2)

	// Fixed backoff
	d3 := router.calculateBackoff(5*time.Second, &types.RetryConfig{
		Backoff: "fixed",
	})
	assert.Equal(t, 5*time.Second, d3)

	// Default fallback with 0 maxDelay (defaults to 30s)
	d4 := router.calculateBackoff(2*time.Second, &types.RetryConfig{
		Backoff:  "unknown_strategy",
		MaxDelay: 0,
	})
	assert.Equal(t, 4*time.Second, d4)
}

// TestRouter_CompareValues_And_ConditionMatching tests comparisons, null checks, and operators.
func TestRouter_CompareValues_And_ConditionMatching(t *testing.T) {
	router := NewRouter(nil)

	// compareValues
	assert.Equal(t, 0, router.compareValues(10.0, 10.0))
	assert.Equal(t, 1, router.compareValues(15.0, 10.0))
	assert.Equal(t, -1, router.compareValues(5.0, 10.0))
	assert.Equal(t, 0, router.compareValues("same", "same"))
	assert.Equal(t, -1, router.compareValues("alpha", "beta"))
	assert.Equal(t, 1, router.compareValues("zebra", "beta"))

	msg := types.Message{
		EventType: "job.event",
		Payload: map[string]any{
			"status":   "running",
			"score":    95.5,
			"tags":     []any{"production", "critical"},
			"null_val": nil,
		},
		Metadata: map[string]string{
			"env": "prod",
		},
	}

	// isNull condition
	assert.True(t, router.evaluateCondition(msg, Condition{Field: "null_val", Operator: "isNull"}))
	assert.False(t, router.evaluateCondition(msg, Condition{Field: "status", Operator: "isNull"}))
	assert.True(t, router.evaluateCondition(msg, Condition{Field: "missing_field", Operator: "isNull"}))

	// regex condition
	assert.True(t, router.evaluateCondition(msg, Condition{Field: "status", Operator: "regex", Value: "^run.*"}))
	assert.False(t, router.evaluateCondition(msg, Condition{Field: "status", Operator: "regex", Value: "^stopped$"}))
	assert.False(t, router.evaluateCondition(msg, Condition{Field: "score", Operator: "regex", Value: ".*"})) // non-string

	// contains condition non-string
	assert.False(t, router.evaluateCondition(msg, Condition{Field: "score", Operator: "contains", Value: "95"}))
	assert.False(t, router.evaluateCondition(msg, Condition{Field: "status", Operator: "contains", Value: 123}))

	// in condition
	assert.True(t, router.evaluateCondition(msg, Condition{Field: "env", Operator: "in", Value: []any{"prod", "staging"}}))
	assert.False(t, router.evaluateCondition(msg, Condition{Field: "env", Operator: "in", Value: []any{"dev", "test"}}))
	assert.False(t, router.evaluateCondition(msg, Condition{Field: "env", Operator: "in", Value: "not_a_slice"}))

	// Matcher metadata filters
	m1 := MessageMatcher{JobCategory: "missing_category"}
	assert.False(t, router.matches(msg, m1))

	m2 := MessageMatcher{JobType: "missing_type"}
	assert.False(t, router.matches(msg, m2))

	m3 := MessageMatcher{Severity: "missing_severity"}
	assert.False(t, router.matches(msg, m3))

	m4 := MessageMatcher{JobID: "missing_job_id"}
	assert.False(t, router.matches(msg, m4))
}

// TestRouterMetrics_RatesZeroAndSnapshot covers zero rates and dropped metrics.
func TestRouterMetrics_RatesZeroAndSnapshot(t *testing.T) {
	metrics := NewRouterMetrics()

	// Snapshot with 0 routed and 0 executed
	snap0 := metrics.GetSnapshot()
	assert.Equal(t, 100.0, snap0.GetSuccessRate())
	assert.Equal(t, 0.0, snap0.GetMatchRate())

	// Record success and evaluation
	metrics.RecordMessageRouted()
	metrics.RecordMessageMatched()
	metrics.RecordRuleEvaluated()
	metrics.RecordRuleMatched()
	metrics.RecordActionExecuted("test_proto", true)

	snap1 := metrics.GetSnapshot()
	assert.Equal(t, 100.0, snap1.GetSuccessRate())
	assert.Equal(t, 100.0, snap1.GetMatchRate())

	// Record a failure: 1 success out of 2 executions -> 50%
	metrics.RecordActionExecuted("test_proto", false)
	snap2 := metrics.GetSnapshot()
	assert.Equal(t, 50.0, snap2.GetSuccessRate())
}
