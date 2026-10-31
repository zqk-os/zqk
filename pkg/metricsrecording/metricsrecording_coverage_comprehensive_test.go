package metricsrecording

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestComprehensive_DecRefEdgeCases(t *testing.T) {
	// Test decRef directly on various integer values
	var val int32 = 0
	decRef(&val)
	assert.Equal(t, int32(0), atomic.LoadInt32(&val), "decRef on 0 should remain 0")

	val = -10
	decRef(&val)
	assert.Equal(t, int32(-10), atomic.LoadInt32(&val), "decRef on negative should remain unchanged")

	val = 3
	decRef(&val)
	assert.Equal(t, int32(2), atomic.LoadInt32(&val), "decRef on 3 should decrement to 2")
	decRef(&val)
	assert.Equal(t, int32(1), atomic.LoadInt32(&val), "decRef on 2 should decrement to 1")
	decRef(&val)
	assert.Equal(t, int32(0), atomic.LoadInt32(&val), "decRef on 1 should decrement to 0")
	decRef(&val)
	assert.Equal(t, int32(0), atomic.LoadInt32(&val), "decRef on 0 should remain 0")
}

func TestComprehensive_IsTruthy(t *testing.T) {
	truthyCases := []string{
		"1",
		"true",
		"True",
		"TRUE",
		"tRuE",
	}
	for _, tc := range truthyCases {
		t.Run("Truthy_"+tc, func(t *testing.T) {
			assert.True(t, isTruthy(tc), "expected %q to be truthy", tc)
		})
	}

	falsyCases := []string{
		"",
		"0",
		"false",
		"False",
		"FALSE",
		"no",
		"yes",
		"2",
		"-1",
		" truth ",
		"true1",
	}
	for _, tc := range falsyCases {
		t.Run("Falsy_"+tc, func(t *testing.T) {
			assert.False(t, isTruthy(tc), "expected %q to be falsy", tc)
		})
	}
}

func TestComprehensive_KindManagementLifecycle(t *testing.T) {
	ClearDeniedKinds()
	t.Cleanup(ClearDeniedKinds)

	EnterAllowRecording()
	t.Cleanup(LeaveAllowRecording)

	// Ensure empty kind does nothing
	DenyKind("")
	assert.True(t, EnabledForKind(""))

	// Allow on non-existent kind should be safe
	AllowKind("non_existent_kind")
	AllowKind("")

	// Add multiple denied kinds
	kinds := []string{"kind_alpha", "kind_beta", "kind_gamma"}
	for _, k := range kinds {
		DenyKind(k)
	}

	for _, k := range kinds {
		assert.False(t, EnabledForKind(k), "kind %s should be denied", k)
	}
	assert.True(t, EnabledForKind("kind_unregistered"), "unregistered kind should be allowed")
	assert.True(t, EnabledForKind(""), "empty kind should be allowed when recording enabled")

	// Allow one back
	AllowKind("kind_beta")
	assert.False(t, EnabledForKind("kind_alpha"))
	assert.True(t, EnabledForKind("kind_beta"))
	assert.False(t, EnabledForKind("kind_gamma"))

	// Clear all denied kinds
	ClearDeniedKinds()
	for _, k := range kinds {
		assert.True(t, EnabledForKind(k), "kind %s should be allowed after ClearDeniedKinds", k)
	}
}

func TestComprehensive_NestedRecordingScopes(t *testing.T) {
	// Reset state safely
	for atomic.LoadInt32(&allowRecordingRef) > 0 {
		LeaveAllowRecording()
	}
	for atomic.LoadInt32(&hotPathNoPersistRef) > 0 {
		LeaveHotPathNoPersist()
	}

	t.Setenv(zqkenv.TestMetricsRecording().Name(), "")

	assert.False(t, Enabled(), "initially disabled")

	// Scope 1
	EnterAllowRecording()
	assert.True(t, Enabled(), "enabled after 1st enter")

	// Scope 2 (nested)
	EnterAllowRecording()
	assert.True(t, Enabled(), "enabled after 2nd enter")

	// Scope 3 (nested)
	EnterAllowRecording()
	assert.True(t, Enabled(), "enabled after 3rd enter")

	// Leave Scope 3
	LeaveAllowRecording()
	assert.True(t, Enabled(), "still enabled after leaving 3rd")

	// Leave Scope 2
	LeaveAllowRecording()
	assert.True(t, Enabled(), "still enabled after leaving 2nd")

	// Leave Scope 1
	LeaveAllowRecording()
	assert.False(t, Enabled(), "disabled after leaving 1st")

	// Extra Leave calls should not cause negative refcount or crash
	LeaveAllowRecording()
	LeaveAllowRecording()
	assert.False(t, Enabled(), "disabled after extra leaves")
	assert.Equal(t, int32(0), atomic.LoadInt32(&allowRecordingRef))
}

func TestComprehensive_NestedHotPathScopes(t *testing.T) {
	// Reset state safely
	for atomic.LoadInt32(&allowRecordingRef) > 0 {
		LeaveAllowRecording()
	}
	for atomic.LoadInt32(&hotPathNoPersistRef) > 0 {
		LeaveHotPathNoPersist()
	}

	t.Setenv(zqkenv.TestMetricsRecording().Name(), "")

	EnterAllowRecording()
	t.Cleanup(LeaveAllowRecording)
	assert.True(t, Enabled())

	// Suppress via HotPath
	EnterHotPathNoPersist()
	assert.False(t, Enabled(), "suppressed after 1st hot path enter")

	// Nested HotPath
	EnterHotPathNoPersist()
	assert.False(t, Enabled(), "suppressed after 2nd hot path enter")

	// Leave 1st HotPath
	LeaveHotPathNoPersist()
	assert.False(t, Enabled(), "still suppressed after 1st hot path leave")

	// Leave 2nd HotPath
	LeaveHotPathNoPersist()
	assert.True(t, Enabled(), "restored after all hot path scopes left")

	// Extra leaves are safe
	LeaveHotPathNoPersist()
	LeaveHotPathNoPersist()
	assert.True(t, Enabled(), "still enabled after extra hot path leaves")
	assert.Equal(t, int32(0), atomic.LoadInt32(&hotPathNoPersistRef))
}

func TestComprehensive_HotPathOverridesAllOptIns(t *testing.T) {
	// Reset state safely
	for atomic.LoadInt32(&allowRecordingRef) > 0 {
		LeaveAllowRecording()
	}
	for atomic.LoadInt32(&hotPathNoPersistRef) > 0 {
		LeaveHotPathNoPersist()
	}

	EnterAllowRecording()
	t.Cleanup(LeaveAllowRecording)
	t.Setenv(zqkenv.TestMetricsRecording().Name(), "1")

	// With both env and refcount active, hot path must strictly suppress
	EnterHotPathNoPersist()
	defer LeaveHotPathNoPersist()

	assert.False(t, Enabled(), "hot path must override env opt-in and refcount")
	assert.False(t, EnabledForKind("some_kind"), "EnabledForKind must be false when hot path is active")
	assert.False(t, EnabledForKind(""), "EnabledForKind for empty kind must be false when hot path is active")
}

func TestComprehensive_CollectAndResetDetailed(t *testing.T) {
	// When disabled:
	EnterHotPathNoPersist()
	collectCalled := false
	resetCalled := false

	res, err := CollectAndReset(func() (string, error) {
		collectCalled = true
		return "should-not-run", nil
	}, func() {
		resetCalled = true
	})

	assert.NoError(t, err)
	assert.Empty(t, res)
	assert.False(t, collectCalled)
	assert.False(t, resetCalled)
	LeaveHotPathNoPersist()

	// When enabled:
	EnterAllowRecording()
	t.Cleanup(LeaveAllowRecording)

	// Subtest 1: Successful collection with reset
	collectCalled = false
	resetCalled = false
	expectedID := "metric-xyz-123"

	res, err = CollectAndReset(func() (string, error) {
		collectCalled = true
		return expectedID, nil
	}, func() {
		resetCalled = true
	})

	assert.NoError(t, err)
	assert.Equal(t, expectedID, res)
	assert.True(t, collectCalled)
	assert.True(t, resetCalled)

	// Subtest 2: Successful collection without reset callback (nil)
	collectCalled = false
	res, err = CollectAndReset(func() (string, error) {
		collectCalled = true
		return "metric-no-reset", nil
	}, nil)

	assert.NoError(t, err)
	assert.Equal(t, "metric-no-reset", res)
	assert.True(t, collectCalled)

	// Subtest 3: Error during collection
	errBoom := errors.New("storage write failed")
	resetCalled = false
	res, err = CollectAndReset(func() (string, error) {
		return "", errBoom
	}, func() {
		resetCalled = true
	})

	assert.ErrorIs(t, err, errBoom)
	assert.Empty(t, res)
	assert.False(t, resetCalled, "reset must not be called when collect returns error")
}

func TestComprehensive_ConcurrentStressSafety(t *testing.T) {
	ClearDeniedKinds()
	t.Cleanup(ClearDeniedKinds)

	const goroutines = 50
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines * 4)

	// Worker group 1: Allow/Leave Recording
	for i := 0; i < goroutines; i++ {
		workerID := i
		goroutinelabels.NewGoroutine("test_metricsrecording_allow_worker", fmt.Sprintf("allow-worker-%d", workerID)).StartSimple(func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				EnterAllowRecording()
				_ = Enabled()
				LeaveAllowRecording()
			}
		})
	}

	// Worker group 2: HotPath Enter/Leave
	for i := 0; i < goroutines; i++ {
		workerID := i
		goroutinelabels.NewGoroutine("test_metricsrecording_hotpath_worker", fmt.Sprintf("hotpath-worker-%d", workerID)).StartSimple(func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				EnterHotPathNoPersist()
				_ = Enabled()
				LeaveHotPathNoPersist()
			}
		})
	}

	// Worker group 3: DenyKind / AllowKind / ClearDeniedKinds
	for i := 0; i < goroutines; i++ {
		workerID := i
		goroutinelabels.NewGoroutine("test_metricsrecording_kind_worker", fmt.Sprintf("kind-worker-%d", workerID)).StartSimple(func() {
			defer wg.Done()
			kind := fmt.Sprintf("test_kind_%d", workerID)
			for j := 0; j < iterations; j++ {
				DenyKind(kind)
				_ = EnabledForKind(kind)
				AllowKind(kind)
				_ = EnabledForKind(kind)
				if j%20 == 0 {
					ClearDeniedKinds()
				}
			}
		})
	}

	// Worker group 4: CollectAndReset concurrently
	for i := 0; i < goroutines; i++ {
		workerID := i
		goroutinelabels.NewGoroutine("test_metricsrecording_collect_worker", fmt.Sprintf("collect-worker-%d", workerID)).StartSimple(func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_, _ = CollectAndReset(func() (string, error) {
					return "metric-stress", nil
				}, func() {})
			}
		})
	}

	wg.Wait()

	// Ensure clean state after concurrency stress
	ClearDeniedKinds()
	for atomic.LoadInt32(&allowRecordingRef) > 0 {
		LeaveAllowRecording()
	}
	for atomic.LoadInt32(&hotPathNoPersistRef) > 0 {
		LeaveHotPathNoPersist()
	}

	assert.Equal(t, int32(0), atomic.LoadInt32(&allowRecordingRef))
	assert.Equal(t, int32(0), atomic.LoadInt32(&hotPathNoPersistRef))
}
