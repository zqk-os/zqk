package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/storage"
)

const storageInitBudget = 2 * time.Second

// TestNewFileObjectStorage_InitCompletesWithinBudget ensures storage init (EnsureReady only,
// no full spec pre-warm) completes within budget. Regression test for OBJECT_OPERATIONS_PERFORMANCE:
// init must not do ReadDir + LoadSpecWithInheritance for every spec on every CLI run.
func TestNewFileObjectStorage_InitCompletesWithinBudget(t *testing.T) {
	testRoot := storage.SetupStorageInitBudgetTestEnvironmentForTest(t)

	start := time.Now()
	st, err := storage.NewFileObjectStorageForTest(testRoot)

	defer func() { _ = st.Shutdown(context.Background()) }()
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("NewFileObjectStorageForTest: %v", err)
	}
	if st != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = st.Shutdown(ctx)
	}
	if elapsed > storageInitBudget {
		t.Errorf("storage init took %v; must complete within %v (OBJECT_OPERATIONS_PERFORMANCE)", elapsed, storageInitBudget)
	}
}

// BenchmarkNewFileObjectStorage_Init reports init time (EnsureReady + validator/caches).
// Run: go test -bench=BenchmarkNewFileObjectStorage_Init -benchtime=3s ./pkg/storage/
func BenchmarkNewFileObjectStorage_Init(b *testing.B) {
	b.StopTimer()
	testRoot := b.TempDir()
	storage.MustBenchmarkSetupTestRootWithLayoutAndSpecs(b, testRoot)
	b.Cleanup(func() {
		_ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(testRoot, nil))
	})
	b.StartTimer()

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		st, err := storage.NewFileObjectStorageForTest(testRoot)
		if err != nil {
			b.Fatal(err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = st.Shutdown(ctx)
		cancel()
	}
}
