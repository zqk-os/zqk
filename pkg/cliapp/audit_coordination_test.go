package cli

import (
	"context"
	"testing"
)

func TestInitAuditCoordination_AndList(t *testing.T) {
	ctx := context.Background()
	meta := map[string]any{"source": "test", "attempt": 1}

	// InitAuditCoordination
	resCtx, coord, resMeta := InitAuditCoordination(ctx, "/test/root", nil, "human", meta)
	if resCtx == nil {
		t.Errorf("expected non-nil context")
	}
	if coord == nil {
		t.Errorf("expected non-nil coordinator")
	}
	if len(resMeta) != 2 || resMeta["source"] != "test" {
		t.Errorf("expected audit metadata copy, got: %v", resMeta)
	}

	// InitListCoordination with empty or dot project root -> ok=false
	_, _, ok := InitListCoordination(ctx, "", "human")
	if ok {
		t.Errorf("expected ok=false for empty project root")
	}
	_, _, ok = InitListCoordination(ctx, ".", "human")
	if ok {
		t.Errorf("expected ok=false for dot project root")
	}

	// InitListCoordination with valid project root -> ok=true
	listCtx, listCoord, ok := InitListCoordination(ctx, "/valid/project/root", "human")
	if !ok || listCtx == nil || listCoord == nil {
		t.Errorf("expected valid coordinator from InitListCoordination, got ok=%v", ok)
	}
}
