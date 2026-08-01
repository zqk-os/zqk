package storage

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// TestWALVisibilitySampling provides an evidence-backed measurement of:
// how long it takes (from writer.Create returning) until a separate reader
// can successfully reader.Read() the new object.
//
// This is intentionally polling at a constant interval so we can estimate
// responsiveness and later tune the underlying durability barriers.
func TestWALVisibilitySampling(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow WAL visibility sampling test in short mode")
	}
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot(), tmpDir)
	t.Cleanup(func() {
		if err := RunProjectTestTeardown(TempProjectTeardown(tmpDir, nil)); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	setupTestRootLikeSetupTestEnvironmentWithSpecsOrSkip(t, tmpDir)

	// Writer: write-behind enabled.
	writer, err := NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFileObjectStorage (writer): %v", err)
	}
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = writer.Shutdown(shutdownCtx)
	})
	if writer.writeBuf == nil || writer.wal == nil || writer.writeBehindWorker == nil {
		t.Fatalf("write-behind not enabled (writeBuf/wal/worker nil)")
	}

	// Reader: write-behind disabled (disk reads).
	reader, err := NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("NewFileObjectStorageForTest (reader): %v", err)
	}
	defer func() { _ = reader.Shutdown(context.Background()) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = reader.Shutdown(shutdownCtx)
	}()

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	const runs = 10
	const sampleInterval = 20 * time.Millisecond
	const perRunTimeout = 8 * time.Second

	// Phase 1: raw visibility sampling (fixed-interval polling).
	rawLatencies := make([]time.Duration, 0, runs)
	rawFailures := 0
	for i := 0; i < runs; i++ {
		id := fmt.Sprintf("ITEM-WB-VIS-%03d", i)
		obj := minimalBacklogItemForWriteBehind(id)
		createCtx := pkgctx.WithCacheUpdate(ctx, id, "backlog_item", "")

		start := time.Now()
		if err := writer.Create(createCtx, secCtx, obj); err != nil {
			t.Fatalf("Create (writer) id=%s: %v", id, err)
		}

		deadline := time.Now().Add(perRunTimeout)
		for {
			_, readErr := reader.Read(ctx, secCtx, id)
			if readErr == nil {
				rawLatencies = append(rawLatencies, time.Since(start))
				break
			}
			if time.Now().After(deadline) {
				rawFailures++
				t.Logf("visibility timeout id=%s after=%s lastErr=%v", id, perRunTimeout, readErr)
				break
			}
			time.Sleep(sampleInterval)
		}
	}

	sort.Slice(rawLatencies, func(i, j int) bool { return rawLatencies[i] < rawLatencies[j] })

	rawSuccess := len(rawLatencies)
	var rawAvg time.Duration
	for _, d := range rawLatencies {
		rawAvg += d
	}
	if rawSuccess > 0 {
		rawAvg /= time.Duration(rawSuccess)
	}

	rawP50 := time.Duration(0)
	rawP90 := time.Duration(0)
	if rawSuccess > 0 {
		rawP50 = rawLatencies[int(float64(rawSuccess-1)*0.50)]
		rawP90 = rawLatencies[int(float64(rawSuccess-1)*0.90)]
	}

	var rawMax time.Duration
	if rawSuccess > 0 {
		rawMax = rawLatencies[len(rawLatencies)-1]
	}

	t.Logf("WAL visibility sampling (raw): runs=%d success=%d failures=%d interval=%s perRunTimeout=%s avg=%s p50=%s p90=%s max=%s",
		runs, rawSuccess, rawFailures,
		sampleInterval, perRunTimeout,
		rawAvg, rawP50, rawP90, rawMax,
	)

	if rawFailures > 0 {
		t.Fatalf("visibility sampling (raw) had %d failures (see logs above)", rawFailures)
	}

	// Phase 2: durability barrier sampling.
	// This measures how long the cross-process durability barrier takes to return,
	// using EnsureCLIObjectMutationVisibleForProvider(nil, ...), and then how quickly
	// the reader can read without additional waiting.
	ensureLatencies := make([]time.Duration, 0, runs) // total (wait + flush) latency
	waitDurations := make([]time.Duration, 0, runs)   // WAL checkpoint wait portion
	flushDurations := make([]time.Duration, 0, runs)  // CAS listing index flush portion
	ensureReadFailures := 0
	extraReadWaits := make([]time.Duration, 0, runs) // reader wait after barrier returns

	for i := 0; i < runs; i++ {
		id := fmt.Sprintf("ITEM-WB-VIS-ENS-%03d", i)
		obj := minimalBacklogItemForWriteBehind(id)
		createCtx := pkgctx.WithCacheUpdate(ctx, id, "backlog_item", "")

		if err := writer.Create(createCtx, secCtx, obj); err != nil {
			t.Fatalf("Create (writer) id=%s: %v", id, err)
		}

		flushCtx, cancel := DurabilityFlushContext()
		startEnsure := time.Now() // include both wait + flush

		// Mirror EnsureCLIObjectMutationVisibleForProvider(nil provider) logic so we can
		// measure the sub-step bottlenecks without changing production code.
		waitTimeout := 15 * time.Second
		if flushCtx != nil {
			if dl, ok := flushCtx.Deadline(); ok {
				if remaining := time.Until(dl); remaining > 0 && remaining < waitTimeout {
					waitTimeout = remaining
				}
			}
		}

		startWait := time.Now()
		err := WaitForWALProcessingEventDriven(tmpDir, waitTimeout)
		waitLatency := time.Since(startWait)

		cancel()
		if err != nil {
			t.Fatalf("EnsureCLIObjectMutationVisibleForProvider id=%s: %v", id, err)
		}

		startFlush := time.Now()
		if err := FlushListingIndexForProjectRoot(tmpDir, "backlog_item"); err != nil {
			t.Fatalf("FlushListingIndexForProjectRoot id=%s: %v", id, err)
		}
		flushLatency := time.Since(startFlush)

		ensureLatencies = append(ensureLatencies, time.Since(startEnsure))
		waitDurations = append(waitDurations, waitLatency)
		flushDurations = append(flushDurations, flushLatency)

		deadline := time.Now().Add(perRunTimeout)
		startRead := time.Now()
		for {
			_, readErr := reader.Read(ctx, secCtx, id)
			if readErr == nil {
				extraReadWaits = append(extraReadWaits, time.Since(startRead))
				break
			}
			if time.Now().After(deadline) {
				ensureReadFailures++
				t.Logf("ensure read timeout id=%s after=%s lastErr=%v", id, perRunTimeout, readErr)
				break
			}
			time.Sleep(sampleInterval)
		}
	}

	sort.Slice(ensureLatencies, func(i, j int) bool { return ensureLatencies[i] < ensureLatencies[j] })
	sort.Slice(waitDurations, func(i, j int) bool { return waitDurations[i] < waitDurations[j] })
	sort.Slice(flushDurations, func(i, j int) bool { return flushDurations[i] < flushDurations[j] })
	sort.Slice(extraReadWaits, func(i, j int) bool { return extraReadWaits[i] < extraReadWaits[j] })

	ensureSuccess := len(ensureLatencies)
	var ensureAvg time.Duration
	for _, d := range ensureLatencies {
		ensureAvg += d
	}
	if ensureSuccess > 0 {
		ensureAvg /= time.Duration(ensureSuccess)
	}

	ensureP50 := time.Duration(0)
	ensureP90 := time.Duration(0)
	if ensureSuccess > 0 {
		ensureP50 = ensureLatencies[int(float64(ensureSuccess-1)*0.50)]
		ensureP90 = ensureLatencies[int(float64(ensureSuccess-1)*0.90)]
	}

	var ensureMax time.Duration
	if ensureSuccess > 0 {
		ensureMax = ensureLatencies[len(ensureLatencies)-1]
	}

	extraSuccess := len(extraReadWaits)
	var extraAvg time.Duration
	for _, d := range extraReadWaits {
		extraAvg += d
	}
	if extraSuccess > 0 {
		extraAvg /= time.Duration(extraSuccess)
	}

	extraP50 := time.Duration(0)
	extraP90 := time.Duration(0)
	if extraSuccess > 0 {
		extraP50 = extraReadWaits[int(float64(extraSuccess-1)*0.50)]
		extraP90 = extraReadWaits[int(float64(extraSuccess-1)*0.90)]
	}

	var extraMax time.Duration
	if extraSuccess > 0 {
		extraMax = extraReadWaits[len(extraReadWaits)-1]
	}

	// Summarize wait + flush sub-step medians.
	waitAvg := time.Duration(0)
	for _, d := range waitDurations {
		waitAvg += d
	}
	if ensureSuccess > 0 {
		waitAvg /= time.Duration(ensureSuccess)
	}

	flushAvg := time.Duration(0)
	for _, d := range flushDurations {
		flushAvg += d
	}
	if ensureSuccess > 0 {
		flushAvg /= time.Duration(ensureSuccess)
	}

	t.Logf("WAL visibility sampling (ensure barrier): runs=%d success=%d readFailures=%d avg=%s p50=%s p90=%s max=%s; post-ensure read wait avg=%s p50=%s p90=%s max=%s",
		runs, ensureSuccess, ensureReadFailures,
		ensureAvg, ensureP50, ensureP90, ensureMax,
		extraAvg, extraP50, extraP90, extraMax,
	)
	t.Logf("WAL visibility sampling (ensure barrier breakdown): wait(avg=%s p50=%s p90=%s max=%s), flush(avg=%s p50=%s p90=%s max=%s)",
		waitAvg,
		func() time.Duration {
			if ensureSuccess <= 0 {
				return 0
			}
			return waitDurations[int(float64(ensureSuccess-1)*0.50)]
		}(),
		func() time.Duration {
			if ensureSuccess <= 0 {
				return 0
			}
			return waitDurations[int(float64(ensureSuccess-1)*0.90)]
		}(),
		func() time.Duration {
			if ensureSuccess <= 0 {
				return 0
			}
			return waitDurations[len(waitDurations)-1]
		}(),
		flushAvg,
		func() time.Duration {
			if ensureSuccess <= 0 {
				return 0
			}
			return flushDurations[int(float64(ensureSuccess-1)*0.50)]
		}(),
		func() time.Duration {
			if ensureSuccess <= 0 {
				return 0
			}
			return flushDurations[int(float64(ensureSuccess-1)*0.90)]
		}(),
		func() time.Duration {
			if ensureSuccess <= 0 {
				return 0
			}
			return flushDurations[len(flushDurations)-1]
		}(),
	)

	if ensureReadFailures > 0 {
		t.Fatalf("visibility sampling (ensure barrier) had %d read failures (see logs above)", ensureReadFailures)
	}
}
