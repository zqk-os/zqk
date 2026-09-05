package hts_test

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/infrastructure/hts"
	"github.com/lanceman/zqk/pkg/objects"
)

func TestDataCell_AssemblyAndVerification(t *testing.T) {
	ctx := context.Background()

	// 1. Define objects for the cell
	skill := map[string]any{
		objects.FieldKeyID:    "SKL-GO-ARCHITECT",
		objects.FieldKeyKind:  objects.KindAgentSkill,
		objects.FieldKeyTitle: "Go Architecture Expert",
	}

	req := map[string]any{
		objects.FieldKeyID:    "REQ-401",
		objects.FieldKeyKind:  objects.KindRequirement,
		objects.FieldKeyTitle: "HTS Data Cell Assembly",
	}

	items := []map[string]any{skill, req}

	// 2. Assemble the cell
	cell, err := hts.Assemble(ctx, "CELL-001", items)
	if err != nil {
		t.Fatalf("failed to assemble cell: %v", err)
	}

	if cell.RootHash == "" {
		t.Errorf("expected RootHash to be populated")
	}

	// 3. Verify the cell (Success)
	if err := cell.Verify(); err != nil {
		t.Errorf("verification failed on intact cell: %v", err)
	}

	// 4. Tamper with an object and verify (Failure)
	cell.Objects["REQ-401"][objects.FieldKeyTitle] = "TAMPERED TITLE"
	if err := cell.Verify(); err == nil {
		t.Errorf("expected verification failure on tampered cell, but it passed")
	}
}
