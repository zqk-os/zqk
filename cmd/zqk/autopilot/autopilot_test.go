package autopilot

import (
	"context"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/storage"
)

func TestAutopilot_StaticFloor_OptionSchemaAndValidation(t *testing.T) {
	cmd := NewAutopilotCmd()
	if cmd.Use != "autopilot" {
		t.Fatalf("expected Use 'autopilot', got %q", cmd.Use)
	}

	flags := []string{"intent", "plan-id", "model", "budget-tokens", "verify", "dry-run"}
	for _, f := range flags {
		if cmd.Flags().Lookup(f) == nil {
			t.Errorf("missing expected flag: %s", f)
		}
	}
}

func TestAutopilot_NegativeAdversarialBoundary_EmptyIntentAndPlan(t *testing.T) {
	ctx := context.Background()
	sec := &pkgctx.SecurityContext{AccountID: "ACC-TEST"}
	tmpDir := t.TempDir()
	store, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to init test storage: %v", err)
	}

	opts := AutopilotOptions{
		Intent:       "",
		PlanID:       "",
		BudgetTokens: 1000,
	}

	execErr := ExecuteAutopilot(ctx, sec, store, opts, nil)
	if execErr == nil {
		t.Fatal("expected error for empty intent and plan, got nil")
	}
}

func TestAutopilot_NegativeAdversarialBoundary_NonPositiveBudget(t *testing.T) {
	ctx := context.Background()
	sec := &pkgctx.SecurityContext{AccountID: "ACC-TEST"}
	tmpDir := t.TempDir()
	store, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to init test storage: %v", err)
	}

	opts := AutopilotOptions{
		Intent:       "Build feature",
		BudgetTokens: 0,
	}

	execErr := ExecuteAutopilot(ctx, sec, store, opts, nil)
	if execErr == nil {
		t.Fatal("expected error for non-positive budget, got nil")
	}
}

func TestAutopilot_OperationalDynamicProof_ValidExecution(t *testing.T) {
	ctx := context.Background()
	sec := &pkgctx.SecurityContext{AccountID: "ACC-TEST"}
	tmpDir := t.TempDir()
	store, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to init test storage: %v", err)
	}

	opts := AutopilotOptions{
		Intent:       "Scaffold feature XYZ",
		BudgetTokens: 50000,
		Verify:       true,
		DryRun:       true,
	}

	if err := ExecuteAutopilot(ctx, sec, store, opts, nil); err != nil {
		t.Fatalf("expected clean execution, got: %v", err)
	}
}
