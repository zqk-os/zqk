package ambient

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// TestBackpressureRegulator_StaticFloorInvariants verifies static floor invariants,
// bounded accumulation floors, and strict nil-safety.
func TestBackpressureRegulator_StaticFloorInvariants(t *testing.T) {
	t.Run("DefaultConfigValid", func(t *testing.T) {
		cfg := DefaultElasticCapacityConfig()
		if err := cfg.Validate(); err != nil {
			t.Fatalf("expected DefaultElasticCapacityConfig to be valid, got: %v", err)
		}
	})

	t.Run("NilSafety", func(t *testing.T) {
		var nilLimiter *TokenBucketLimiter
		if nilLimiter.TryAcquire(1) {
			t.Errorf("nil limiter TryAcquire must return false")
		}
		if nilLimiter.AvailableTokens() != 0.0 {
			t.Errorf("nil limiter AvailableTokens must return 0.0")
		}
		if nilLimiter.Capacity() != 0.0 {
			t.Errorf("nil limiter Capacity must return 0.0")
		}
		if nilLimiter.RefillRate() != 0.0 {
			t.Errorf("nil limiter RefillRate must return 0.0")
		}
		if err := nilLimiter.SetCapacity(10); err != ErrNilLimiter {
			t.Errorf("expected ErrNilLimiter, got: %v", err)
		}
		if err := nilLimiter.SetRefillRate(10); err != ErrNilLimiter {
			t.Errorf("expected ErrNilLimiter, got: %v", err)
		}

		var nilReg *BackpressureRegulator
		if _, err := nilReg.ScaleCapacity(0.5); err != ErrNilRegulator {
			t.Errorf("expected ErrNilRegulator, got: %v", err)
		}
		if err := nilReg.RecordArrival(); err != ErrNilRegulator {
			t.Errorf("expected ErrNilRegulator, got: %v", err)
		}
		if err := nilReg.RecordArrivalAt(time.Now()); err != ErrNilRegulator {
			t.Errorf("expected ErrNilRegulator, got: %v", err)
		}
		if nilReg.CurrentThroughput() != 0.0 {
			t.Errorf("nil reg CurrentThroughput must return 0.0")
		}
		if dec, _, err := nilReg.EvaluateAdmission(1); err != ErrNilRegulator || dec != AdmissionReject {
			t.Errorf("nil reg EvaluateAdmission must return ErrNilRegulator and AdmissionReject, got (%v, %v)", dec, err)
		}
		if nilReg.CurrentCapacity() != 0 {
			t.Errorf("nil reg CurrentCapacity must return 0")
		}
		if nilReg.AvailableTokens() != 0.0 {
			t.Errorf("nil reg AvailableTokens must return 0.0")
		}
		if err := nilReg.Close(); err != ErrNilRegulator {
			t.Errorf("expected ErrNilRegulator on Close, got: %v", err)
		}
		if !nilReg.IsClosed() {
			t.Errorf("nil reg IsClosed must return true")
		}
	})

	t.Run("BoundedTokenAccumulationFloor", func(t *testing.T) {
		limiter, err := NewTokenBucketLimiter(10.0, 100.0)
		if err != nil {
			t.Fatalf("unexpected limiter error: %v", err)
		}

		time.Sleep(50 * time.Millisecond)
		avail := limiter.AvailableTokens()
		if avail > 10.0 {
			t.Errorf("tokens must not exceed capacity (10.0), got %f", avail)
		}
	})
}

// TestBackpressureRegulator_OperationalDynamicProof satisfies CRIT-1793604737814892000-4c199bdf:
// dynamic throughput calculation, adaptive capacity scaling from 1x to 10x swarm baseline,
// and non-blocking admission throttling.
func TestBackpressureRegulator_OperationalDynamicProof(t *testing.T) {
	t.Run("DynamicThroughputCalculationAccuracyUnderBursts", func(t *testing.T) {
		cfg := DefaultElasticCapacityConfig()
		cfg.WindowDuration = 500 * time.Millisecond
		reg, err := NewBackpressureRegulator(cfg)
		if err != nil {
			t.Fatalf("failed to create regulator: %v", err)
		}
		defer func() {
			if closeErr := reg.Close(); closeErr != nil {
				t.Errorf("close failed: %v", closeErr)
			}
		}()

		now := time.Now()
		// Record 25 arrivals within the current window
		for i := 0; i < 25; i++ {
			if recErr := reg.RecordArrivalAt(now.Add(-time.Duration(i*10) * time.Millisecond)); recErr != nil {
				t.Fatalf("RecordArrivalAt failed: %v", recErr)
			}
		}

		tp := reg.CurrentThroughput()
		// 25 arrivals in 0.5s window => 50 arrivals/sec
		expectedTp := 25.0 / 0.5
		if tp < expectedTp-1.0 || tp > expectedTp+1.0 {
			t.Errorf("expected throughput around %f, got %f", expectedTp, tp)
		}

		// Also verify RecordArrival() with current time
		if err := reg.RecordArrival(); err != nil {
			t.Fatalf("RecordArrival failed: %v", err)
		}
	})

	t.Run("AdaptiveCapacityScaling", func(t *testing.T) {
		cfg := ElasticCapacityConfig{
			BaselineCapacity:      50,
			MaxCapacityMultiplier: 4.0, // max = 200
			RefillRate:            100.0,
			WindowDuration:        1 * time.Second,
			LoadSheddingThreshold: 0.90,
			MinBackoff:            10 * time.Millisecond,
			MaxBackoff:            500 * time.Millisecond,
			BackoffFactor:         2.0,
		}

		reg, err := NewBackpressureRegulator(cfg)
		if err != nil {
			t.Fatalf("failed to create regulator: %v", err)
		}
		defer func() {
			if closeErr := reg.Close(); closeErr != nil {
				t.Errorf("close failed: %v", closeErr)
			}
		}()

		if reg.CurrentCapacity() != 50 {
			t.Errorf("expected initial capacity 50, got %d", reg.CurrentCapacity())
		}

		// Half load: 50% loadRatio -> capacity should scale up to halfway (50 + 0.5*(200-50)) = 125
		capHalf, err := reg.ScaleCapacity(0.5)
		if err != nil {
			t.Fatalf("ScaleCapacity(0.5) failed: %v", err)
		}
		if capHalf != 125 {
			t.Errorf("expected capacity 125, got %d", capHalf)
		}
		if reg.CurrentCapacity() != 125 {
			t.Errorf("expected CurrentCapacity() 125, got %d", reg.CurrentCapacity())
		}

		// Full saturation load: 1.0 -> capacity should scale up to 4x baseline = 200
		capFull, err := reg.ScaleCapacity(1.0)
		if err != nil {
			t.Fatalf("ScaleCapacity(1.0) failed: %v", err)
		}
		if capFull != 200 {
			t.Errorf("expected capacity 200, got %d", capFull)
		}

		// Over-saturation ratio (> 1.0) is clamped to max capacity (200)
		capOver, err := reg.ScaleCapacity(2.5)
		if err != nil {
			t.Fatalf("ScaleCapacity(2.5) failed: %v", err)
		}
		if capOver != 200 {
			t.Errorf("expected clamped capacity 200, got %d", capOver)
		}

		// Gradual scale-down on idle (0.0) -> returns to baseline capacity (50)
		capIdle, err := reg.ScaleCapacity(0.0)
		if err != nil {
			t.Fatalf("ScaleCapacity(0.0) failed: %v", err)
		}
		if capIdle != 50 {
			t.Errorf("expected capacity to scale down to 50 on idle, got %d", capIdle)
		}
	})

	t.Run("AdmissionThrottlingAndBackoffLoops", func(t *testing.T) {
		cfg := ElasticCapacityConfig{
			BaselineCapacity:      10,
			MaxCapacityMultiplier: 2.0, // max = 20
			RefillRate:            1.0, // very slow refill to test throttling
			WindowDuration:        1 * time.Second,
			LoadSheddingThreshold: 0.80, // saturation >= 80% sheds load
			MinBackoff:            20 * time.Millisecond,
			MaxBackoff:            200 * time.Millisecond,
			BackoffFactor:         2.0,
		}

		reg, err := NewBackpressureRegulator(cfg)
		if err != nil {
			t.Fatalf("failed to create regulator: %v", err)
		}
		defer func() {
			if closeErr := reg.Close(); closeErr != nil {
				t.Errorf("close failed: %v", closeErr)
			}
		}()

		// Step 1: Allow when tokens are available
		dec, waitDur, err := reg.EvaluateAdmission(2)
		if err != nil {
			t.Fatalf("EvaluateAdmission(2) failed: %v", err)
		}
		if dec != AdmissionAllow || waitDur != 0 {
			t.Errorf("expected AdmissionAllow with 0 wait, got (%s, %v)", dec, waitDur)
		}

		// Step 2: Consume more tokens without reaching shedding threshold (tokens now ~8, cost 5 -> remaining ~3, saturation ~70% < 80%)
		dec2, _, err := reg.EvaluateAdmission(5)
		if err != nil {
			t.Fatalf("EvaluateAdmission(5) failed: %v", err)
		}
		if dec2 != AdmissionAllow {
			t.Errorf("expected AdmissionAllow, got %s", dec2)
		}

		// Step 3: Consume tokens to exceed available tokens (cost 5 with only ~3 available)
		// But saturation is 70% < 80%, so it should return AdmissionWait with a valid backoff duration
		dec3, waitDur3, err := reg.EvaluateAdmission(5)
		if err != nil {
			t.Fatalf("EvaluateAdmission(5) failed: %v", err)
		}
		if dec3 != AdmissionWait {
			t.Errorf("expected AdmissionWait, got %s", dec3)
		}
		if waitDur3 < cfg.MinBackoff || waitDur3 > cfg.MaxBackoff {
			t.Errorf("expected wait duration between %v and %v, got %v", cfg.MinBackoff, cfg.MaxBackoff, waitDur3)
		}

		// Step 4: Deplete bucket completely so saturation >= LoadSheddingThreshold (80%)
		// Drain remaining tokens
		reg.limiter.TryAcquire(3)
		// Now available tokens = 0, saturation = 1.0 >= 0.80 -> AdmissionReject (load shedding)
		dec4, waitDur4, err := reg.EvaluateAdmission(1)
		if err != nil {
			t.Fatalf("EvaluateAdmission(1) failed: %v", err)
		}
		if dec4 != AdmissionReject || waitDur4 != 0 {
			t.Errorf("expected AdmissionReject with 0 wait, got (%s, %v)", dec4, waitDur4)
		}
	})

	t.Run("BackoffCalculationBoundedWithJitter", func(t *testing.T) {
		cfg := DefaultElasticCapacityConfig()
		cfg.MinBackoff = 50 * time.Millisecond
		cfg.MaxBackoff = 800 * time.Millisecond
		cfg.BackoffFactor = 2.0

		reg, err := NewBackpressureRegulator(cfg)
		if err != nil {
			t.Fatalf("failed to create regulator: %v", err)
		}
		defer func() {
			if closeErr := reg.Close(); closeErr != nil {
				t.Errorf("close failed: %v", closeErr)
			}
		}()

		b0 := reg.CalculateBackoff(0)
		if b0 < cfg.MinBackoff || b0 > cfg.MaxBackoff {
			t.Errorf("expected b0 in [%v, %v], got %v", cfg.MinBackoff, cfg.MaxBackoff, b0)
		}

		b1 := reg.CalculateBackoff(1)
		if b1 < b0 {
			t.Errorf("expected b1 >= b0, got b0=%v, b1=%v", b0, b1)
		}

		bLarge := reg.CalculateBackoff(50)
		if bLarge > cfg.MaxBackoff {
			t.Errorf("expected bLarge to be capped at maxBackoff %v, got %v", cfg.MaxBackoff, bLarge)
		}
	})
}

// TestBackpressureRegulator_NegativeBoundaries satisfies CRIT-1793604740585565000-1e4c6668:
// rejection of malformed limits, negative bounds, burst exhaustion, and concurrent race safety.
func TestBackpressureRegulator_NegativeBoundaries(t *testing.T) {
	t.Run("ConfigValidationFailures", func(t *testing.T) {
		badConfigs := []ElasticCapacityConfig{
			{BaselineCapacity: 0, MaxCapacityMultiplier: 2.0, RefillRate: 10, WindowDuration: time.Second, LoadSheddingThreshold: 0.9, MinBackoff: time.Millisecond, MaxBackoff: time.Second, BackoffFactor: 2.0},
			{BaselineCapacity: 10, MaxCapacityMultiplier: 0.5, RefillRate: 10, WindowDuration: time.Second, LoadSheddingThreshold: 0.9, MinBackoff: time.Millisecond, MaxBackoff: time.Second, BackoffFactor: 2.0},
			{BaselineCapacity: 10, MaxCapacityMultiplier: 2.0, RefillRate: 0, WindowDuration: time.Second, LoadSheddingThreshold: 0.9, MinBackoff: time.Millisecond, MaxBackoff: time.Second, BackoffFactor: 2.0},
			{BaselineCapacity: 10, MaxCapacityMultiplier: 2.0, RefillRate: 10, WindowDuration: 0, LoadSheddingThreshold: 0.9, MinBackoff: time.Millisecond, MaxBackoff: time.Second, BackoffFactor: 2.0},
			{BaselineCapacity: 10, MaxCapacityMultiplier: 2.0, RefillRate: 10, WindowDuration: time.Second, LoadSheddingThreshold: 0.0, MinBackoff: time.Millisecond, MaxBackoff: time.Second, BackoffFactor: 2.0},
			{BaselineCapacity: 10, MaxCapacityMultiplier: 2.0, RefillRate: 10, WindowDuration: time.Second, LoadSheddingThreshold: 1.5, MinBackoff: time.Millisecond, MaxBackoff: time.Second, BackoffFactor: 2.0},
			{BaselineCapacity: 10, MaxCapacityMultiplier: 2.0, RefillRate: 10, WindowDuration: time.Second, LoadSheddingThreshold: 0.9, MinBackoff: 2 * time.Second, MaxBackoff: time.Second, BackoffFactor: 2.0},
			{BaselineCapacity: 10, MaxCapacityMultiplier: 2.0, RefillRate: 10, WindowDuration: time.Second, LoadSheddingThreshold: 0.9, MinBackoff: time.Millisecond, MaxBackoff: time.Second, BackoffFactor: 0.5},
		}

		for idx, badCfg := range badConfigs {
			if err := badCfg.Validate(); err == nil {
				t.Errorf("case %d: expected error for invalid config, got nil", idx)
			}
			if _, err := NewBackpressureRegulator(badCfg); err == nil {
				t.Errorf("case %d: expected NewBackpressureRegulator error, got nil", idx)
			}
		}
	})

	t.Run("LimiterConstructorErrors", func(t *testing.T) {
		if _, err := NewTokenBucketLimiter(0, 10); err != ErrInvalidCapacity {
			t.Errorf("expected ErrInvalidCapacity, got %v", err)
		}
		if _, err := NewTokenBucketLimiter(10, 0); err != ErrInvalidRefillRate {
			t.Errorf("expected ErrInvalidRefillRate, got %v", err)
		}
	})

	t.Run("InvalidOperationalArguments", func(t *testing.T) {
		reg, err := NewBackpressureRegulator(DefaultElasticCapacityConfig())
		if err != nil {
			t.Fatalf("failed to create regulator: %v", err)
		}
		defer func() {
			if closeErr := reg.Close(); closeErr != nil {
				t.Errorf("close failed: %v", closeErr)
			}
		}()

		if _, err := reg.ScaleCapacity(-0.1); err != ErrInvalidLoadRatio {
			t.Errorf("expected ErrInvalidLoadRatio, got: %v", err)
		}

		if _, _, err := reg.EvaluateAdmission(0); err != ErrInvalidCost {
			t.Errorf("expected ErrInvalidCost, got: %v", err)
		}

		if _, _, err := reg.EvaluateAdmission(-5); err != ErrInvalidCost {
			t.Errorf("expected ErrInvalidCost, got: %v", err)
		}

		maxCap := reg.maxCapacity
		if _, _, err := reg.EvaluateAdmission(maxCap + 1); err != ErrCostExceedsCapacity {
			t.Errorf("expected ErrCostExceedsCapacity, got: %v", err)
		}
	})

	t.Run("ClosedRegulatorRejection", func(t *testing.T) {
		reg, err := NewBackpressureRegulator(DefaultElasticCapacityConfig())
		if err != nil {
			t.Fatalf("failed to create regulator: %v", err)
		}

		if err := reg.Close(); err != nil {
			t.Fatalf("close failed: %v", err)
		}
		if !reg.IsClosed() {
			t.Errorf("expected IsClosed() to be true")
		}

		// Repeat close is idempotent
		if err := reg.Close(); err != nil {
			t.Errorf("repeat close should return nil, got: %v", err)
		}

		if _, err := reg.ScaleCapacity(0.5); err != ErrRegulatorClosed {
			t.Errorf("expected ErrRegulatorClosed on ScaleCapacity, got: %v", err)
		}
		if err := reg.RecordArrival(); err != ErrRegulatorClosed {
			t.Errorf("expected ErrRegulatorClosed on RecordArrival, got: %v", err)
		}
		if err := reg.RecordArrivalAt(time.Now()); err != ErrRegulatorClosed {
			t.Errorf("expected ErrRegulatorClosed on RecordArrivalAt, got: %v", err)
		}
		if _, _, err := reg.EvaluateAdmission(1); err != ErrRegulatorClosed {
			t.Errorf("expected ErrRegulatorClosed on EvaluateAdmission, got: %v", err)
		}
	})

	t.Run("ConcurrentWorkerRaceFreedom", func(t *testing.T) {
		cfg := ElasticCapacityConfig{
			BaselineCapacity:      100,
			MaxCapacityMultiplier: 5.0,
			RefillRate:            500.0,
			WindowDuration:        1 * time.Second,
			LoadSheddingThreshold: 0.90,
			MinBackoff:            5 * time.Millisecond,
			MaxBackoff:            100 * time.Millisecond,
			BackoffFactor:         1.5,
		}

		reg, err := NewBackpressureRegulator(cfg)
		if err != nil {
			t.Fatalf("failed to create regulator: %v", err)
		}
		defer func() {
			if closeErr := reg.Close(); closeErr != nil {
				t.Errorf("close failed: %v", closeErr)
			}
		}()

		const numWorkers = 64
		const iterations = 50
		var wg sync.WaitGroup
		var allowCount int64
		var waitCount int64
		var rejectCount int64

		wg.Add(numWorkers)
		for w := 0; w < numWorkers; w++ {
			workerID := w
			goroutinelabels.NewGoroutine("test_backpressure_worker", "concurrent worker test").StartSimple(func() {
				defer wg.Done()
				for i := 0; i < iterations; i++ {
					// Concurrently record arrivals
					if recErr := reg.RecordArrival(); recErr != nil {
						t.Errorf("worker %d RecordArrival failed: %v", workerID, recErr)
					}

					// Dynamic scaling
					if i%10 == 0 {
						ratio := float64(i%5) / 4.0
						if _, scaleErr := reg.ScaleCapacity(ratio); scaleErr != nil {
							t.Errorf("worker %d ScaleCapacity failed: %v", workerID, scaleErr)
						}
					}

					// Admission check
					dec, _, admErr := reg.EvaluateAdmission(1)
					if admErr != nil {
						t.Errorf("worker %d EvaluateAdmission failed: %v", workerID, admErr)
					}
					switch dec {
					case AdmissionAllow:
						atomic.AddInt64(&allowCount, 1)
					case AdmissionWait:
						atomic.AddInt64(&waitCount, 1)
					case AdmissionReject:
						atomic.AddInt64(&rejectCount, 1)
					}

					// Intermittent backoff or saturation query
					_ = reg.CurrentThroughput()
					_ = reg.CurrentSaturation()
				}
			})
		}

		wg.Wait()
		totalDecisions := atomic.LoadInt64(&allowCount) + atomic.LoadInt64(&waitCount) + atomic.LoadInt64(&rejectCount)
		expectedDecisions := int64(numWorkers * iterations)
		if totalDecisions != expectedDecisions {
			t.Errorf("expected %d total decisions, got %d (allow=%d, wait=%d, reject=%d)",
				expectedDecisions, totalDecisions, allowCount, waitCount, rejectCount)
		}
	})
}
