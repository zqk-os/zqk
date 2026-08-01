package testscan

import (
	"sync"
	"testing"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

func TestComputePackageConcurrencyLimitsFromBundles_AllParallelUsesMaxParallel(t *testing.T) {
	t.Parallel()
	bundles := []*TestBundle{
		{
			PackagePath: "pkg/foo",
			Tests: []*TestFunction{
				{Name: "A", IsParallel: true},
				{Name: "B", IsParallel: true},
			},
		},
	}
	got := ComputePackageConcurrencyLimitsFromBundles(bundles, 6)
	if got["pkg/foo"] != 6 {
		t.Fatalf("got %v", got)
	}
}

func TestComputePackageConcurrencyLimitsFromBundles_SequentialTestForcesOne(t *testing.T) {
	t.Parallel()
	bundles := []*TestBundle{
		{
			PackagePath: "pkg/foo",
			Tests: []*TestFunction{
				{Name: "A", IsParallel: true},
				{Name: "B", IsParallel: false},
			},
		},
	}
	got := ComputePackageConcurrencyLimitsFromBundles(bundles, 8)
	if got["pkg/foo"] != 1 {
		t.Fatalf("got %v want 1", got)
	}
}

func TestComputePackageConcurrencyLimitsFromBundles_SchedulerPackagePolicy(t *testing.T) {
	t.Parallel()
	bundles := []*TestBundle{
		{
			PackagePath: "cmd/zqk/scheduler",
			Tests: []*TestFunction{
				{Name: "A", IsParallel: true},
			},
		},
	}
	got := ComputePackageConcurrencyLimitsFromBundles(bundles, 8)
	if got["cmd/zqk/scheduler"] != 1 {
		t.Fatalf("got %v want 1", got)
	}
}

func TestWritePackageConcurrencyLimitsPatch_ConcurrentNoRenameRace(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	bundles := []*TestBundle{
		{
			PackagePath: "pkg/foo",
			Tests: []*TestFunction{
				{Name: "A", IsParallel: true},
			},
		},
	}
	const n = 48
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("testscan_test", "concurrent write package concurrency limits patch").StartSimple(func() {
			defer wg.Done()
			errCh <- WritePackageConcurrencyLimitsPatch(root, bundles, 4)
		})
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
}
