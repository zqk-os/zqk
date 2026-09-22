package batchaf

import (
	"context"
	"testing"
)

func TestProcessOrphanedRequirements_ExtraCoverage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Empty list
	if err := ProcessOrphanedRequirements(ctx, nil); err != nil {
		t.Fatalf("expected nil for empty requirements, got %v", err)
	}

	// Non-empty requirements
	reqs := []string{"REQ-1", "REQ-2", "REQ-3"}
	if err := ProcessOrphanedRequirements(ctx, reqs); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	// Direct calls to internal helpers
	if err := parseIdentity("REQ-DIRECT"); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if err := applySystemGovernance("REQ-DIRECT"); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if err := performSemanticTranslation("REQ-DIRECT"); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	// Error branches in ProcessOrphanedRequirements
	if err := ProcessOrphanedRequirements(ctx, []string{""}); err == nil {
		t.Error("expected error for empty req identity")
	}
	if err := ProcessOrphanedRequirements(ctx, []string{"invalid-governance"}); err == nil {
		t.Error("expected error for invalid governance")
	}
	if err := ProcessOrphanedRequirements(ctx, []string{"invalid-translation"}); err == nil {
		t.Error("expected error for invalid translation")
	}

	// Context cancellation
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ProcessOrphanedRequirements(canceledCtx, nil); err == nil {
		t.Error("expected error for canceled context before loop")
	}
	if err := ProcessOrphanedRequirements(canceledCtx, []string{"REQ-1"}); err == nil {
		t.Error("expected error for canceled context in loop")
	}
}
