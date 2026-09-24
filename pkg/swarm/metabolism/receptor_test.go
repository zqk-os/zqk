package metabolism

import (
	"errors"
	"sync"
	"testing"
)

func TestComputeHolonURN_Deterministic(t *testing.T) {
	params1 := map[string]interface{}{
		"baseline":    4.5,
		"concurrency": 4,
		"framework":   "CEF",
	}
	// Same params in different map order
	params2 := map[string]interface{}{
		"framework":   "CEF",
		"baseline":    4.5,
		"concurrency": 4,
	}

	urn1, err := ComputeHolonURN("code-eval", "1.0.0", "sha256:abcd1234ef567890", params1)
	if err != nil {
		t.Fatalf("ComputeHolonURN failed: %v", err)
	}

	urn2, err := ComputeHolonURN("code-eval", "1.0.0", "sha256:abcd1234ef567890", params2)
	if err != nil {
		t.Fatalf("ComputeHolonURN failed: %v", err)
	}

	if urn1.String() != urn2.String() {
		t.Errorf("expected deterministic URNs, got %q vs %q", urn1.String(), urn2.String())
	}

	parsed, err := ParseHolonURN(urn1.String())
	if err != nil {
		t.Fatalf("ParseHolonURN failed: %v", err)
	}
	if parsed.Name != "code-eval" || parsed.Version.String() != "1.0.0" {
		t.Errorf("parsed URN mismatch: %+v", parsed)
	}
}

func TestComputeHolonURN_InvalidSemVerFailsClosed(t *testing.T) {
	_, err := ComputeHolonURN("code-eval", "vFoo3.14", "sha256:1234", nil)
	if err == nil {
		t.Fatal("expected error on non-semver, got nil")
	}
}

func TestReceptorRegistry_SaturationMutexBlocksConcurrent(t *testing.T) {
	reg := NewReceptorRegistry()
	urn := "urn:zqk:pack:code-eval:1.0.0:12345678abcdef00"

	lease1, err := reg.Acquire(urn, "session-alpha")
	if err != nil {
		t.Fatalf("first Acquire failed: %v", err)
	}
	if lease1.Epoch != 1 {
		t.Errorf("expected epoch 1, got %d", lease1.Epoch)
	}

	// Concurrent attempt on the same URN while session-alpha is active
	_, err = reg.Acquire(urn, "session-beta")
	if err == nil {
		t.Fatal("expected ErrReceptorSaturated on concurrent acquire, got nil")
	}
	if !errors.Is(err, ErrReceptorSaturated) {
		t.Errorf("expected ErrReceptorSaturated, got %v", err)
	}

	// Concurrent acquire on a DIFFERENT URN should succeed
	otherURN := "urn:zqk:pack:other-eval:1.0.0:abcdef1234567890"
	otherLease, err := reg.Acquire(otherURN, "session-gamma")
	if err != nil {
		t.Fatalf("acquire on different URN failed: %v", err)
	}
	if otherLease.Epoch != 1 {
		t.Errorf("expected epoch 1 on different URN, got %d", otherLease.Epoch)
	}
}

func TestReceptorRegistry_HistoricalLineageReconciliation(t *testing.T) {
	reg := NewReceptorRegistry()
	urn := "urn:zqk:pack:code-eval:1.0.0:12345678abcdef00"

	// First execution cycle
	lease1, err := reg.Acquire(urn, "session-1")
	if err != nil {
		t.Fatalf("first Acquire failed: %v", err)
	}
	if err := reg.Close(urn, lease1.LeaseID, "converged"); err != nil {
		t.Fatalf("Close lease1 failed: %v", err)
	}

	// Re-ingest after completion: should reconcile lineage and increment epoch
	lease2, err := reg.Acquire(urn, "session-2")
	if err != nil {
		t.Fatalf("second Acquire failed after close: %v", err)
	}
	if lease2.Epoch != 2 {
		t.Errorf("expected epoch 2 on re-ingestion, got %d", lease2.Epoch)
	}
	if err := reg.Close(urn, lease2.LeaseID, "re-verified"); err != nil {
		t.Fatalf("Close lease2 failed: %v", err)
	}

	// Check historical lineage preservation
	lineage, ok := reg.GetLineage(urn)
	if !ok {
		t.Fatal("expected lineage to exist")
	}
	if lineage.Epoch != 2 {
		t.Errorf("expected lineage epoch 2, got %d", lineage.Epoch)
	}
	if len(lineage.History) != 2 {
		t.Fatalf("expected 2 historical ticks, got %d", len(lineage.History))
	}
	if lineage.History[0].Outcome != "converged" || lineage.History[1].Outcome != "re-verified" {
		t.Errorf("unexpected historical outcomes: %+v", lineage.History)
	}
}

func TestReceptorRegistry_ConcurrencyHammer(t *testing.T) {
	reg := NewReceptorRegistry()
	urn := "urn:zqk:pack:code-eval:1.0.0:stress-test-fingerprint"

	concurrency := 20
	var wg sync.WaitGroup
	wg.Add(concurrency)

	var successes int
	var saturations int
	var mu sync.Mutex

	for i := 0; i < concurrency; i++ {
		go func(idx int) {
			defer wg.Done()
			_, err := reg.Acquire(urn, "session-hammer")
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				successes++
			} else if errors.Is(err, ErrReceptorSaturated) {
				saturations++
			}
		}(i)
	}

	wg.Wait()

	if successes != 1 {
		t.Errorf("expected exactly 1 successful acquire under concurrent hammer, got %d", successes)
	}
	if saturations != concurrency-1 {
		t.Errorf("expected %d saturation errors, got %d", concurrency-1, saturations)
	}
}
