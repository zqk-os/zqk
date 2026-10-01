package system

import (
	"sort"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/validation"
)

func TestInProcessWarmCheckBenchmark(t *testing.T) {
	projectRoot := cli.ResolveProjectRoot(".")
	if projectRoot == "" {
		t.Fatalf("failed to resolve project root")
	}

	loadCtx := pkgctx.NewSystemContext()

	// Ensure caches (ObjectIDCache and ReverseRefIndex) are warm
	if err := EnsureObjectIDCacheReady(loadCtx, projectRoot, false, nil, nil); err != nil {
		t.Fatalf("failed to prewarm object ID cache: %v", err)
	}

	specLoader := objects.GetGlobalSpecLoader()
	lifecycleLoader := objects.GetGlobalLifecycleLoader()
	validatorRegistry := validation.GetGlobalRegistry()
	validator := validatorRegistry.Get("")
	objectIDCache := GetGlobalObjectIDCache()

	hashRegistryCache := &HashRegistryCacheType{
		cache: make(map[string]storage.HashRegistryProvider),
	}

	cliCtx := cli.ContextForProjectAndProfile(projectRoot, "system")

	// Benchmark warm CheckKindObjectsWithCache for single object validation
	dummyCmd := &cobra.Command{}
	targetID := "CVS-1234567890123456000-abcdef12"
	var durMs []float64

	// Warmup
	_, _, _ = CheckKindObjectsWithCache(cliCtx, loadCtx, dummyCmd, objects.KindBacklogItem, []string{targetID}, specLoader, lifecycleLoader, validator, hashRegistryCache, objectIDCache)

	for i := 0; i < 10; i++ {
		start := time.Now()
		_, _, err := CheckKindObjectsWithCache(cliCtx, loadCtx, dummyCmd, objects.KindBacklogItem, []string{targetID}, specLoader, lifecycleLoader, validator, hashRegistryCache, objectIDCache)
		elapsed := float64(time.Since(start).Microseconds()) / 1000.0
		if err != nil {
			t.Fatalf("single object check failed: %v", err)
		}
		durMs = append(durMs, elapsed)
	}

	sort.Float64s(durMs)
	p50 := durMs[len(durMs)/2]
	p95 := durMs[int(float64(len(durMs))*0.95)]

	t.Logf("=== In-Process Warm Single Object Check Benchmark ===")
	t.Logf("Warm Single Object Check p50: %.3fms (Target <=2000ms)", p50)
	t.Logf("Warm Single Object Check p95: %.3fms (Target <=2000ms)", p95)

	if p50 > 2000.0 {
		t.Errorf("In-process single object warm check p50 %.3fms exceeded 2000ms gate", p50)
	}
}
