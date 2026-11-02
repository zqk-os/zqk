package ambient

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// TestFaultIsolator_StaticFloorInvariants verifies structural invariants,
// default configuration semantics, parameter validations, zero-allocation lookups,
// and strict nil safety.
func TestFaultIsolator_StaticFloorInvariants(t *testing.T) {
	t.Run("DefaultConfigValid", func(t *testing.T) {
		cfg := DefaultFaultIsolatorConfig()
		if err := cfg.Validate(); err != nil {
			t.Fatalf("expected DefaultFaultIsolatorConfig to be valid, got: %v", err)
		}
		if cfg.FailureRateThreshold != DefaultFailureRateThreshold {
			t.Errorf("expected FailureRateThreshold %v, got %v", DefaultFailureRateThreshold, cfg.FailureRateThreshold)
		}
		if cfg.MinSamplesThreshold != DefaultMinSamplesThreshold {
			t.Errorf("expected MinSamplesThreshold %v, got %v", DefaultMinSamplesThreshold, cfg.MinSamplesThreshold)
		}
		if cfg.WindowDuration != DefaultFaultIsolatorWindowDuration {
			t.Errorf("expected WindowDuration %v, got %v", DefaultFaultIsolatorWindowDuration, cfg.WindowDuration)
		}
		if cfg.CoolOffDuration != DefaultCoolOffDuration {
			t.Errorf("expected CoolOffDuration %v, got %v", DefaultCoolOffDuration, cfg.CoolOffDuration)
		}
		if cfg.HalfOpenMaxProbes != DefaultHalfOpenMaxProbes {
			t.Errorf("expected HalfOpenMaxProbes %v, got %v", DefaultHalfOpenMaxProbes, cfg.HalfOpenMaxProbes)
		}
		if cfg.ConsecutiveSuccessesReset != DefaultConsecutiveSuccessesReset {
			t.Errorf("expected ConsecutiveSuccessesReset %v, got %v", DefaultConsecutiveSuccessesReset, cfg.ConsecutiveSuccessesReset)
		}
		if cfg.NumBuckets != DefaultNumBuckets {
			t.Errorf("expected NumBuckets %v, got %v", DefaultNumBuckets, cfg.NumBuckets)
		}
	})

	t.Run("ConfigValidationFailures", func(t *testing.T) {
		tests := []struct {
			name        string
			modify      func(c *FaultIsolatorConfig)
			expectedErr error
		}{
			{
				name:        "ZeroFailureRateThreshold",
				modify:      func(c *FaultIsolatorConfig) { c.FailureRateThreshold = 0.0 },
				expectedErr: ErrInvalidThreshold,
			},
			{
				name:        "NegativeFailureRateThreshold",
				modify:      func(c *FaultIsolatorConfig) { c.FailureRateThreshold = -0.1 },
				expectedErr: ErrInvalidThreshold,
			},
			{
				name:        "ExceedingFailureRateThreshold",
				modify:      func(c *FaultIsolatorConfig) { c.FailureRateThreshold = 1.1 },
				expectedErr: ErrInvalidThreshold,
			},
			{
				name:        "ZeroMinSamples",
				modify:      func(c *FaultIsolatorConfig) { c.MinSamplesThreshold = 0 },
				expectedErr: ErrInvalidMinSamples,
			},
			{
				name:        "ZeroWindowDuration",
				modify:      func(c *FaultIsolatorConfig) { c.WindowDuration = 0 },
				expectedErr: ErrInvalidWindowDuration,
			},
			{
				name:        "ZeroCoolOffDuration",
				modify:      func(c *FaultIsolatorConfig) { c.CoolOffDuration = 0 },
				expectedErr: ErrInvalidCoolOffDuration,
			},
			{
				name:        "ZeroHalfOpenMaxProbes",
				modify:      func(c *FaultIsolatorConfig) { c.HalfOpenMaxProbes = 0 },
				expectedErr: ErrInvalidHalfOpenProbes,
			},
			{
				name:        "ZeroConsecutiveSuccessesReset",
				modify:      func(c *FaultIsolatorConfig) { c.ConsecutiveSuccessesReset = 0 },
				expectedErr: ErrInvalidConsecutiveSuccess,
			},
			{
				name:        "ZeroNumBuckets",
				modify:      func(c *FaultIsolatorConfig) { c.NumBuckets = 0 },
				expectedErr: ErrInvalidBuckets,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				cfg := DefaultFaultIsolatorConfig()
				tc.modify(&cfg)
				err := cfg.Validate()
				if !errors.Is(err, tc.expectedErr) {
					t.Errorf("expected error %v, got %v", tc.expectedErr, err)
				}
				if _, newErr := NewFaultIsolator(cfg); !errors.Is(newErr, tc.expectedErr) {
					t.Errorf("expected NewFaultIsolator to fail with %v, got %v", tc.expectedErr, newErr)
				}
			})
		}
	})

	t.Run("CircuitBreakerStateConstants", func(t *testing.T) {
		if StateClosed != "Closed" || StateClosed.String() != "Closed" {
			t.Errorf("unexpected StateClosed representation: %s", StateClosed)
		}
		if StateOpen != "Open" || StateOpen.String() != "Open" {
			t.Errorf("unexpected StateOpen representation: %s", StateOpen)
		}
		if StateHalfOpen != "HalfOpen" || StateHalfOpen.String() != "HalfOpen" {
			t.Errorf("unexpected StateHalfOpen representation: %s", StateHalfOpen)
		}
	})

	t.Run("NilSafety", func(t *testing.T) {
		var nilIso *FaultIsolator
		if err := nilIso.Allow(); !errors.Is(err, ErrNilFaultIsolator) {
			t.Errorf("expected ErrNilFaultIsolator from nil Allow(), got: %v", err)
		}
		if err := nilIso.RecordResult(true); !errors.Is(err, ErrNilFaultIsolator) {
			t.Errorf("expected ErrNilFaultIsolator from nil RecordResult(), got: %v", err)
		}
		if err := nilIso.Record(true); !errors.Is(err, ErrNilFaultIsolator) {
			t.Errorf("expected ErrNilFaultIsolator from nil Record(), got: %v", err)
		}
		if err := nilIso.RecordSuccess(); !errors.Is(err, ErrNilFaultIsolator) {
			t.Errorf("expected ErrNilFaultIsolator from nil RecordSuccess(), got: %v", err)
		}
		if err := nilIso.RecordFailure(); !errors.Is(err, ErrNilFaultIsolator) {
			t.Errorf("expected ErrNilFaultIsolator from nil RecordFailure(), got: %v", err)
		}
		if s := nilIso.State(); s != StateOpen {
			t.Errorf("expected nil State() to fail-closed with StateOpen, got: %v", s)
		}
		if cfg := nilIso.Config(); cfg != (FaultIsolatorConfig{}) {
			t.Errorf("expected nil Config() to return zero config, got: %+v", cfg)
		}
		if err := nilIso.Reset(); !errors.Is(err, ErrNilFaultIsolator) {
			t.Errorf("expected ErrNilFaultIsolator from nil Reset(), got: %v", err)
		}
		if err := nilIso.Trip(); !errors.Is(err, ErrNilFaultIsolator) {
			t.Errorf("expected ErrNilFaultIsolator from nil Trip(), got: %v", err)
		}
		if err := nilIso.ForceState(StateClosed); !errors.Is(err, ErrNilFaultIsolator) {
			t.Errorf("expected ErrNilFaultIsolator from nil ForceState(), got: %v", err)
		}
		if nilIso.ActiveProbes() != 0 {
			t.Errorf("expected nil ActiveProbes() 0, got %d", nilIso.ActiveProbes())
		}
		if nilIso.ConsecutiveSuccesses() != 0 {
			t.Errorf("expected nil ConsecutiveSuccesses() 0, got %d", nilIso.ConsecutiveSuccesses())
		}
		if !nilIso.LastStateTransition().IsZero() {
			t.Errorf("expected nil LastStateTransition() to be zero time")
		}
		if nilIso.FailureRate() != 0.0 {
			t.Errorf("expected nil FailureRate() 0.0, got %v", nilIso.FailureRate())
		}
		if nilIso.TotalSamples() != 0 {
			t.Errorf("expected nil TotalSamples() 0, got %d", nilIso.TotalSamples())
		}
		if nilIso.RateTracker() != nil {
			t.Errorf("expected nil RateTracker() nil, got %v", nilIso.RateTracker())
		}

		var nilTracker *SlidingWindowRateTracker
		if err := nilTracker.Record(true); !errors.Is(err, ErrNilRateTracker) {
			t.Errorf("expected ErrNilRateTracker from nil Record(), got: %v", err)
		}
		if err := nilTracker.RecordAt(true, time.Now()); !errors.Is(err, ErrNilRateTracker) {
			t.Errorf("expected ErrNilRateTracker from nil RecordAt(), got: %v", err)
		}
		if nilTracker.FailureRate() != 0.0 {
			t.Errorf("expected nil tracker FailureRate() 0.0, got %v", nilTracker.FailureRate())
		}
		if nilTracker.FailureRateAt(time.Now()) != 0.0 {
			t.Errorf("expected nil tracker FailureRateAt() 0.0, got %v", nilTracker.FailureRateAt(time.Now()))
		}
		if nilTracker.TotalSamples() != 0 {
			t.Errorf("expected nil tracker TotalSamples() 0, got %d", nilTracker.TotalSamples())
		}
		if nilTracker.TotalSamplesAt(time.Now()) != 0 {
			t.Errorf("expected nil tracker TotalSamplesAt() 0, got %d", nilTracker.TotalSamplesAt(time.Now()))
		}
		if s, f := nilTracker.Counts(); s != 0 || f != 0 {
			t.Errorf("expected nil tracker Counts() (0,0), got (%d, %d)", s, f)
		}
		if s, f := nilTracker.CountsAt(time.Now()); s != 0 || f != 0 {
			t.Errorf("expected nil tracker CountsAt() (0,0), got (%d, %d)", s, f)
		}
		if nilTracker.SuccessCount() != 0 {
			t.Errorf("expected nil tracker SuccessCount() 0, got %d", nilTracker.SuccessCount())
		}
		if nilTracker.FailureCount() != 0 {
			t.Errorf("expected nil tracker FailureCount() 0, got %d", nilTracker.FailureCount())
		}
		if err := nilTracker.Prune(time.Now()); !errors.Is(err, ErrNilRateTracker) {
			t.Errorf("expected ErrNilRateTracker from nil Prune(), got: %v", err)
		}
		if err := nilTracker.Reset(); !errors.Is(err, ErrNilRateTracker) {
			t.Errorf("expected ErrNilRateTracker from nil Reset(), got: %v", err)
		}
	})

	t.Run("ZeroAllocationRateLookup", func(t *testing.T) {
		tracker, err := NewSlidingWindowRateTracker(5*time.Second, 10)
		if err != nil {
			t.Fatalf("failed to create tracker: %v", err)
		}
		_ = tracker.Record(true)
		_ = tracker.Record(false)

		allocs := testing.AllocsPerRun(100, func() {
			_ = tracker.FailureRate()
			_ = tracker.TotalSamples()
		})
		if allocs > 0 {
			t.Errorf("expected 0 allocs/op for rate and sample lookups, got %v", allocs)
		}
	})
}

// TestFaultIsolator_OperationalDynamicProof verifies tri-state lifecycle progression,
// adaptive tripping on error bursts, cool-off half-open probe recovery, and automatic reset loops.
func TestFaultIsolator_OperationalDynamicProof(t *testing.T) {
	t.Run("ClosedStateAllowAndErrorTripping", func(t *testing.T) {
		cfg := FaultIsolatorConfig{
			FailureRateThreshold:      0.40,
			MinSamplesThreshold:       5,
			WindowDuration:            2 * time.Second,
			CoolOffDuration:           100 * time.Millisecond,
			HalfOpenMaxProbes:         2,
			ConsecutiveSuccessesReset: 2,
			NumBuckets:                10,
		}
		iso, err := NewFaultIsolator(cfg)
		if err != nil {
			t.Fatalf("failed to create isolator: %v", err)
		}

		if iso.State() != StateClosed {
			t.Fatalf("expected initial state StateClosed, got: %v", iso.State())
		}

		// Initial calls allowed
		if allowErr := iso.Allow(); allowErr != nil {
			t.Fatalf("expected Allow to succeed in Closed state, got: %v", allowErr)
		}

		// Record 3 successes and 1 failure: failure rate = 0.25 (below 0.40, and < 5 samples)
		_ = iso.RecordResult(true)
		_ = iso.RecordResult(true)
		_ = iso.RecordResult(true)
		_ = iso.RecordResult(false)

		if iso.State() != StateClosed {
			t.Fatalf("expected state to remain Closed, got: %v", iso.State())
		}

		// Record 2 more failures: total 6 samples (3 successes, 3 failures) => failure rate = 0.50 (> 0.40)
		_ = iso.RecordResult(false)
		_ = iso.RecordResult(false)

		if iso.State() != StateOpen {
			t.Fatalf("expected state to trip to StateOpen, got: %v", iso.State())
		}

		// In StateOpen, Allow must fail closed immediately
		if err := iso.Allow(); !errors.Is(err, ErrCircuitOpen) {
			t.Fatalf("expected ErrCircuitOpen while in StateOpen, got: %v", err)
		}
	})

	t.Run("CoolOffProgressionToHalfOpenAndResetClosed", func(t *testing.T) {
		cfg := FaultIsolatorConfig{
			FailureRateThreshold:      0.50,
			MinSamplesThreshold:       2,
			WindowDuration:            1 * time.Second,
			CoolOffDuration:           50 * time.Millisecond,
			HalfOpenMaxProbes:         2,
			ConsecutiveSuccessesReset: 2,
			NumBuckets:                10,
		}
		iso, err := NewFaultIsolator(cfg)
		if err != nil {
			t.Fatalf("failed to create isolator: %v", err)
		}

		// Trip circuit to StateOpen
		_ = iso.RecordResult(false)
		_ = iso.RecordResult(false)
		if iso.State() != StateOpen {
			t.Fatalf("expected StateOpen after 2 failures, got: %v", iso.State())
		}

		// Immediately check Allow() returns ErrCircuitOpen
		if err := iso.Allow(); !errors.Is(err, ErrCircuitOpen) {
			t.Fatalf("expected ErrCircuitOpen during cool-off, got: %v", err)
		}

		// Wait for cool-off duration to expire
		time.Sleep(60 * time.Millisecond)

		// State() should now indicate StateHalfOpen
		if state := iso.State(); state != StateHalfOpen {
			t.Fatalf("expected transition to StateHalfOpen after cool-off, got: %v", state)
		}

		// Probe 1 allowed
		if err := iso.Allow(); err != nil {
			t.Fatalf("expected probe 1 to be permitted in HalfOpen, got: %v", err)
		}
		// Probe 2 allowed
		if err := iso.Allow(); err != nil {
			t.Fatalf("expected probe 2 to be permitted in HalfOpen, got: %v", err)
		}
		// Probe 3 rejected (exceeds HalfOpenMaxProbes = 2)
		if err := iso.Allow(); !errors.Is(err, ErrCircuitProbeLimitExceeded) {
			t.Fatalf("expected ErrCircuitProbeLimitExceeded for probe 3, got: %v", err)
		}

		// Record success for probe 1
		if err := iso.RecordResult(true); err != nil {
			t.Fatalf("failed to record probe success: %v", err)
		}
		if iso.State() != StateHalfOpen {
			t.Fatalf("expected state to remain HalfOpen before consecutive threshold, got: %v", iso.State())
		}

		// Record success for probe 2 => consecutive count reaches 2 => resets to StateClosed!
		if err := iso.RecordResult(true); err != nil {
			t.Fatalf("failed to record probe success: %v", err)
		}
		if iso.State() != StateClosed {
			t.Fatalf("expected reset to StateClosed after consecutive successes, got: %v", iso.State())
		}

		// In StateClosed, Allow must succeed
		if err := iso.Allow(); err != nil {
			t.Fatalf("expected Allow to succeed after recovery to Closed, got: %v", err)
		}
	})

	t.Run("HalfOpenProbeFailureTripsToOpenImmediately", func(t *testing.T) {
		cfg := FaultIsolatorConfig{
			FailureRateThreshold:      0.50,
			MinSamplesThreshold:       2,
			WindowDuration:            1 * time.Second,
			CoolOffDuration:           100 * time.Millisecond,
			HalfOpenMaxProbes:         2,
			ConsecutiveSuccessesReset: 3,
			NumBuckets:                10,
		}
		iso, err := NewFaultIsolator(cfg)
		if err != nil {
			t.Fatalf("failed to create isolator: %v", err)
		}

		// Force into HalfOpen
		_ = iso.ForceState(StateHalfOpen)
		if iso.State() != StateHalfOpen {
			t.Fatalf("expected StateHalfOpen, got: %v", iso.State())
		}

		// Permit probe
		if err := iso.Allow(); err != nil {
			t.Fatalf("expected probe permitted, got: %v", err)
		}

		// Probe fails
		_ = iso.RecordResult(false)

		// Must trip immediately back to StateOpen
		if iso.State() != StateOpen {
			t.Fatalf("expected failure in HalfOpen to trip back to StateOpen, got: %v", iso.State())
		}
		if err := iso.Allow(); !errors.Is(err, ErrCircuitOpen) {
			t.Fatalf("expected ErrCircuitOpen after re-tripping to Open, got: %v", err)
		}
	})

	t.Run("ManualTripAndReset", func(t *testing.T) {
		iso := DefaultFaultIsolator()
		if iso.State() != StateClosed {
			t.Fatalf("expected StateClosed, got %v", iso.State())
		}

		if err := iso.Trip(); err != nil {
			t.Fatalf("unexpected error tripping: %v", err)
		}
		if iso.State() != StateOpen {
			t.Fatalf("expected StateOpen after Trip(), got %v", iso.State())
		}

		if err := iso.Reset(); err != nil {
			t.Fatalf("unexpected error resetting: %v", err)
		}
		if iso.State() != StateClosed {
			t.Fatalf("expected StateClosed after Reset(), got %v", iso.State())
		}
	})
}

// TestFaultIsolator_NegativeBoundaries verifies negative adversarial boundaries:
//  1. Zero data races under 60 concurrent worker goroutines executing Allow/Record calls.
//  2. Strict HalfOpen concurrency bounding: 20 concurrent goroutines launch in HalfOpen;
//     exactly HalfOpenMaxProbes obtain access, and remaining receive ErrCircuitProbeLimitExceeded.
//  3. Strict fail-closed rejection during Open state outage windows.
//  4. Memory boundedness under sustained high volume (100,000+ records) with zero memory leaks.
func TestFaultIsolator_NegativeBoundaries(t *testing.T) {
	t.Run("SixtyConcurrentWorkersRaceFreedom", func(t *testing.T) {
		cfg := FaultIsolatorConfig{
			FailureRateThreshold:      0.50,
			MinSamplesThreshold:       50,
			WindowDuration:            500 * time.Millisecond,
			CoolOffDuration:           10 * time.Millisecond,
			HalfOpenMaxProbes:         5,
			ConsecutiveSuccessesReset: 5,
			NumBuckets:                20,
		}
		iso, err := NewFaultIsolator(cfg)
		if err != nil {
			t.Fatalf("failed to create isolator: %v", err)
		}

		const numWorkers = 60
		const duration = 1200 * time.Millisecond
		stopCh := make(chan struct{})

		var wg sync.WaitGroup
		var totalAllowed int64
		var totalOpenRejected int64
		var totalProbeLimited int64
		var totalRecorded int64

		wg.Add(numWorkers)
		for i := 0; i < numWorkers; i++ {
			workerID := i
			goroutinelabels.NewGoroutine("test_fault_isolator_worker", "concurrent race freedom test").StartSimple(func() {
				defer wg.Done()
				iteration := 0
				for {
					select {
					case <-stopCh:
						return
					default:
					}

					iteration++
					allowErr := iso.Allow()
					if allowErr == nil {
						atomic.AddInt64(&totalAllowed, 1)
						// Alternate successes and occasional failures
						success := (workerID+iteration)%4 != 0
						if recErr := iso.RecordResult(success); recErr != nil {
							t.Errorf("worker %d RecordResult failed: %v", workerID, recErr)
						}
						atomic.AddInt64(&totalRecorded, 1)
					} else if errors.Is(allowErr, ErrCircuitOpen) {
						atomic.AddInt64(&totalOpenRejected, 1)
					} else if errors.Is(allowErr, ErrCircuitProbeLimitExceeded) {
						atomic.AddInt64(&totalProbeLimited, 1)
					}

					// Intermittent inspection of state and telemetry
					if iteration%20 == 0 {
						_ = iso.State()
						_ = iso.FailureRate()
						_ = iso.TotalSamples()
						_ = iso.ActiveProbes()
						_ = iso.ConsecutiveSuccesses()
						_ = iso.Config()
					}
				}
			})
		}

		time.Sleep(duration)
		close(stopCh)
		wg.Wait()

		if atomic.LoadInt64(&totalAllowed) == 0 {
			t.Errorf("expected at least some calls to be allowed during concurrent race run")
		}
		if atomic.LoadInt64(&totalRecorded) == 0 {
			t.Errorf("expected at least some calls to be recorded during concurrent race run")
		}
	})

	t.Run("HalfOpenStrictConcurrencyBounding", func(t *testing.T) {
		const maxProbes = 4
		cfg := FaultIsolatorConfig{
			FailureRateThreshold:      0.50,
			MinSamplesThreshold:       10,
			WindowDuration:            2 * time.Second,
			CoolOffDuration:           1 * time.Hour, // do not auto-transition
			HalfOpenMaxProbes:         maxProbes,
			ConsecutiveSuccessesReset: 10,
			NumBuckets:                20,
		}
		iso, err := NewFaultIsolator(cfg)
		if err != nil {
			t.Fatalf("failed to create isolator: %v", err)
		}

		// Force directly into StateHalfOpen
		if forceErr := iso.ForceState(StateHalfOpen); forceErr != nil {
			t.Fatalf("failed to force state: %v", forceErr)
		}

		const numGoroutines = 20
		startGate := make(chan struct{})
		var wg sync.WaitGroup
		var successCount int64
		var limitExceededCount int64
		var otherErrCount int64

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			goroutinelabels.NewGoroutine("test_half_open_bounding", "half-open probe bound test").StartSimple(func() {
				defer wg.Done()
				<-startGate // synchronize launch
				err := iso.Allow()
				if err == nil {
					atomic.AddInt64(&successCount, 1)
				} else if errors.Is(err, ErrCircuitProbeLimitExceeded) {
					atomic.AddInt64(&limitExceededCount, 1)
				} else {
					atomic.AddInt64(&otherErrCount, 1)
				}
			})
		}

		// Release all 20 goroutines simultaneously
		close(startGate)
		wg.Wait()

		if successCount != int64(maxProbes) {
			t.Errorf("expected exactly %d probes to be permitted, got: %d", maxProbes, successCount)
		}
		expectedRejections := int64(numGoroutines - maxProbes)
		if limitExceededCount != expectedRejections {
			t.Errorf("expected exactly %d rejections with ErrCircuitProbeLimitExceeded, got: %d", expectedRejections, limitExceededCount)
		}
		if otherErrCount != 0 {
			t.Errorf("expected 0 unexpected errors, got: %d", otherErrCount)
		}
		if iso.ActiveProbes() != maxProbes {
			t.Errorf("expected ActiveProbes() to be %d, got: %d", maxProbes, iso.ActiveProbes())
		}
	})

	t.Run("FailClosedRejectionDuringOpenOutage", func(t *testing.T) {
		cfg := FaultIsolatorConfig{
			FailureRateThreshold:      0.50,
			MinSamplesThreshold:       5,
			WindowDuration:            2 * time.Second,
			CoolOffDuration:           1 * time.Hour, // long cool-off
			HalfOpenMaxProbes:         3,
			ConsecutiveSuccessesReset: 3,
			NumBuckets:                20,
		}
		iso, err := NewFaultIsolator(cfg)
		if err != nil {
			t.Fatalf("failed to create isolator: %v", err)
		}

		// Trip to StateOpen
		if tripErr := iso.Trip(); tripErr != nil {
			t.Fatalf("failed to trip isolator: %v", tripErr)
		}
		if iso.State() != StateOpen {
			t.Fatalf("expected StateOpen, got: %v", iso.State())
		}

		const numRejections = 30
		var wg sync.WaitGroup
		var openRejectedCount int64

		wg.Add(numRejections)
		for i := 0; i < numRejections; i++ {
			goroutinelabels.NewGoroutine("test_fail_closed_open", "fail-closed open verification").StartSimple(func() {
				defer wg.Done()
				if err := iso.Allow(); errors.Is(err, ErrCircuitOpen) {
					atomic.AddInt64(&openRejectedCount, 1)
				}
			})
		}
		wg.Wait()

		if openRejectedCount != int64(numRejections) {
			t.Errorf("expected all %d requests to be rejected with ErrCircuitOpen, got: %d", numRejections, openRejectedCount)
		}
	})

	t.Run("MemoryBoundednessUnderHighVolume", func(t *testing.T) {
		const numRecords = 100000
		tracker, err := NewSlidingWindowRateTracker(1*time.Second, 60)
		if err != nil {
			t.Fatalf("failed to create rate tracker: %v", err)
		}

		// Force GC before baseline measurement
		runtime.GC()
		var memBefore runtime.MemStats
		runtime.ReadMemStats(&memBefore)

		now := time.Now()
		for i := 0; i < numRecords; i++ {
			// Alternate successes and failures advancing time every 1000 records
			eventTime := now.Add(time.Duration(i/1000) * 10 * time.Millisecond)
			if err := tracker.RecordAt(i%2 == 0, eventTime); err != nil {
				t.Fatalf("unexpected record error: %v", err)
			}
		}

		runtime.GC()
		var memAfter runtime.MemStats
		runtime.ReadMemStats(&memAfter)

		// Verify constant slice capacity and zero bucket expansion
		if cap(tracker.buckets) != 60 || len(tracker.buckets) != 60 {
			t.Errorf("expected constant bucket slice capacity 60, got cap=%d len=%d", cap(tracker.buckets), len(tracker.buckets))
		}

		// Verify memory did not grow linearly with request count (e.g. heap delta < 500KB)
		var heapDelta int64
		if memAfter.HeapAlloc > memBefore.HeapAlloc {
			heapDelta = int64(memAfter.HeapAlloc - memBefore.HeapAlloc)
		}
		// 100,000 requests without bounded buffers would consume many megabytes.
		if heapDelta > 1024*1024 { // 1MB threshold
			t.Errorf("heap growth exceeded bounded budget: %d bytes allocated across %d records", heapDelta, numRecords)
		}

		// Test Prune
		evictTime := now.Add(5 * time.Second)
		if err := tracker.Prune(evictTime); err != nil {
			t.Fatalf("Prune failed: %v", err)
		}
		if tracker.TotalSamplesAt(evictTime) != 0 {
			t.Errorf("expected 0 total samples after pruning past window, got: %d", tracker.TotalSamplesAt(evictTime))
		}
		if tracker.FailureRateAt(evictTime) != 0.0 {
			t.Errorf("expected 0.0 failure rate after pruning, got: %v", tracker.FailureRateAt(evictTime))
		}
	})
}
