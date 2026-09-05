package objectcreate

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/lanceman/zqk/internal/cli"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
)

func TestInProcessWarmCreateBenchmark(t *testing.T) {
	projectRoot := cli.ResolveProjectRoot(".")
	// Option B write-behind storage enabled
	store, err := storage.NewFileObjectStorage(projectRoot)
	if err != nil {
		t.Fatalf("failed to initialize FileObjectStorage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, projectRoot, store)

	secCtx := pkgctx.NewSystemSecurityContext()
	opCtx := pkgctx.NewSystemContext()

	// Warmup create
	warmupObj := map[string]any{
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyTitle:         "Warmup Create Test",
		objects.FieldKeyStatus:        objects.ObjectStatusProposed,
		objects.FieldKeyID:            fmt.Sprintf("BLI-warmup-%d", time.Now().UnixNano()),
	}
	_ = store.Create(opCtx, secCtx, warmupObj)

	// Benchmark N=10 warm creates
	var durMs []float64
	for i := 0; i < 10; i++ {
		testObj := map[string]any{
			objects.FieldKeyKind:          objects.KindBacklogItem,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyTitle:         fmt.Sprintf("Warm Create Test %d", i+1),
			objects.FieldKeyStatus:        objects.ObjectStatusProposed,
			objects.FieldKeyID:            fmt.Sprintf("BLI-bench-%d-%d", time.Now().UnixNano(), i),
		}
		start := time.Now()
		err := store.Create(opCtx, secCtx, testObj)
		elapsed := float64(time.Since(start).Microseconds()) / 1000.0
		if err != nil {
			t.Fatalf("run %d create failed: %v", i+1, err)
		}
		durMs = append(durMs, elapsed)
	}

	sort.Float64s(durMs)
	p50 := durMs[len(durMs)/2]
	p95 := durMs[int(float64(len(durMs))*0.95)]

	t.Logf("=== In-Process Storage Create Warm Benchmark ===")
	t.Logf("Warm p50: %.3fms (Target <=1000ms)", p50)
	t.Logf("Warm p95: %.3fms (Target <=2000ms)", p95)

	if p50 > 1000.0 {
		t.Errorf("In-process create p50 %.3fms exceeded 1000ms gate", p50)
	}
	if p95 > 2000.0 {
		t.Errorf("In-process create p95 %.3fms exceeded 2000ms gate", p95)
	}
}
