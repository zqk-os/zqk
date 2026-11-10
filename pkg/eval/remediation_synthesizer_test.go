package eval

import (
	"context"
	"strings"
	"sync"
	"testing"
)

func TestCycle70_RemediationSynthesizer_StaticFloorInvariants(t *testing.T) {
	t.Parallel()

	t.Run("normalize_error_message_timestamps_and_pointers", func(t *testing.T) {
		raw := "failed at 2026-11-10T01:05:00Z with pointer 0x7ffee124 in (45.2ms): unexpected nil"
		norm := NormalizeErrorMessage(raw)
		if strings.Contains(norm, "2026-11-10") {
			t.Errorf("timestamp was not normalized: %s", norm)
		}
		if strings.Contains(norm, "0x7ffee124") {
			t.Errorf("hex pointer was not normalized: %s", norm)
		}
		if strings.Contains(norm, "45.2ms") {
			t.Errorf("duration was not normalized: %s", norm)
		}
		if !strings.Contains(norm, "unexpected nil") {
			t.Errorf("error core was corrupted: %s", norm)
		}
	})

	t.Run("deterministic_fingerprint_generation", func(t *testing.T) {
		fp1 := ComputeFailureFingerprint("sc-1", "model-a", "assertion failure: want 1 got 0")
		fp2 := ComputeFailureFingerprint("sc-1", "model-a", "assertion failure: want 1 got 0")
		if fp1 != fp2 {
			t.Fatalf("expected identical fingerprints, got %s != %s", fp1, fp2)
		}
		fp3 := ComputeFailureFingerprint("sc-2", "model-a", "assertion failure: want 1 got 0")
		if fp1 == fp3 {
			t.Fatalf("expected distinct fingerprints across scenarios")
		}
	})

	t.Run("classify_severity_mapping", func(t *testing.T) {
		if sev := ClassifySeverity(CategoryPanic); sev != SeverityP0 {
			t.Errorf("expected P0 for panic, got %s", sev)
		}
		if sev := ClassifySeverity(CategoryBoundary); sev != SeverityP0 {
			t.Errorf("expected P0 for boundary, got %s", sev)
		}
		if sev := ClassifySeverity(CategoryAssertion); sev != SeverityP1 {
			t.Errorf("expected P1 for assertion, got %s", sev)
		}
		if sev := ClassifySeverity(CategoryTimeout); sev != SeverityP2 {
			t.Errorf("expected P2 for timeout, got %s", sev)
		}
	})
}

func TestCycle70_RemediationSynthesizer_OperationalDynamicProof(t *testing.T) {
	t.Parallel()

	var mintedItems []RemediationItem
	var mu sync.Mutex

	mockMinter := func(ctx context.Context, item RemediationItem) error {
		mu.Lock()
		defer mu.Unlock()
		mintedItems = append(mintedItems, item)
		return nil
	}

	synth := NewRemediationSynthesizer(mockMinter)
	payload := FailurePayload{
		ScenarioID:   "sc-order-processing",
		ModelName:    "primary-model",
		ErrorMessage: "invariant violated at 2026-11-10T01:10:00Z: balance cannot be negative",
		Category:     CategoryAssertion,
		CodeLocation: "pkg/order/process.go:120",
	}

	res, err := synth.Synthesize(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error during synthesis: %v", err)
	}
	if !res.Minted || res.Deduplicated {
		t.Fatalf("expected novel failure to be minted, got res: %+v", res)
	}
	if res.Severity != SeverityP1 {
		t.Errorf("expected P1 severity, got %s", res.Severity)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(mintedItems) != 1 {
		t.Fatalf("expected 1 minted item, got %d", len(mintedItems))
	}
	if mintedItems[0].ScenarioID != "sc-order-processing" {
		t.Errorf("unexpected scenario ID in item: %s", mintedItems[0].ScenarioID)
	}
}

func TestCycle70_RemediationSynthesizer_NegativeAdversarialBoundary(t *testing.T) {
	t.Parallel()

	synth := NewRemediationSynthesizer(nil)

	t.Run("empty_scenario_id_fails_soft", func(t *testing.T) {
		_, err := synth.Synthesize(context.Background(), FailurePayload{
			ScenarioID:   "",
			ErrorMessage: "valid error",
		})
		if err == nil {
			t.Fatalf("expected error on empty scenario ID")
		}
	})

	t.Run("empty_error_message_fails_soft", func(t *testing.T) {
		_, err := synth.Synthesize(context.Background(), FailurePayload{
			ScenarioID:   "sc-valid",
			ErrorMessage: "   ",
		})
		if err == nil {
			t.Fatalf("expected error on empty error message")
		}
	})

	t.Run("duplicate_failure_is_deduplicated", func(t *testing.T) {
		p := FailurePayload{
			ScenarioID:   "sc-dup-test",
			ModelName:    "test-model",
			ErrorMessage: "fatal lock contention at 0x1234 (12ms)",
			Category:     CategoryPanic,
		}

		res1, err := synth.Synthesize(context.Background(), p)
		if err != nil || !res1.Minted {
			t.Fatalf("first run should mint: err=%v res=%+v", err, res1)
		}

		res2, err := synth.Synthesize(context.Background(), p)
		if err != nil {
			t.Fatalf("second run should not error: %v", err)
		}
		if res2.Minted || !res2.Deduplicated {
			t.Fatalf("expected second run to be deduplicated: %+v", res2)
		}
		if res2.Fingerprint != res1.Fingerprint {
			t.Errorf("fingerprint mismatch across duplicate runs: %s vs %s", res1.Fingerprint, res2.Fingerprint)
		}
	})
}
