package objectget

import (
	"sort"
	"testing"
	"time"

	"github.com/lanceman/zqk/internal/cli"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
)

func TestInProcessWarmGetBenchmark(t *testing.T) {
	projectRoot := cli.ResolveProjectRoot(".")
	store, err := storage.NewFileObjectStorage(projectRoot)
	if err != nil {
		t.Fatalf("failed to initialize FileObjectStorage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, projectRoot, store)

	secCtx := pkgctx.NewSystemSecurityContext()
	opCtx := pkgctx.NewSystemContext()

	targetID := "[REDACTED-ID]"

	// Warmup read (fixture BLI may be absent in minimal checkouts)
	_, err = store.Read(opCtx, secCtx, targetID)
	if err != nil {
		t.Skipf("warmup object %s not in project — skip bench: %v", targetID, err)
	}

	// Benchmark N=10 warm reads
	var durMs []float64
	for i := 0; i < 10; i++ {
		start := time.Now()
		obj, err := store.Read(opCtx, secCtx, targetID)
		elapsed := float64(time.Since(start).Microseconds()) / 1000.0
		if err != nil || obj == nil {
			t.Fatalf("run %d read failed: %v", i+1, err)
		}
		durMs = append(durMs, elapsed)
	}

	// Calculate p50 and p95 on sorted samples (unsorted indexes made p95<p50 spurious).
	sort.Float64s(durMs)
	p50 := durMs[len(durMs)/2]
	p95 := durMs[int(float64(len(durMs))*0.95)]
	if p95 < p50 {
		p95 = durMs[len(durMs)-1]
	}

	t.Logf("=== In-Process Storage Read Warm Benchmark ===")
	t.Logf("Warm p50: %.3fms (Target <=300ms)", p50)
	t.Logf("Warm p95: %.3fms (Target <=800ms)", p95)

	if p50 > 300.0 {
		t.Errorf("In-process p50 %.3fms exceeded 300ms gate", p50)
	}
	if p95 > 800.0 {
		t.Errorf("In-process p95 %.3fms exceeded 800ms gate", p95)
	}
}
