package hts_test

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/infrastructure/crypto"
	"github.com/lanceman/zqk/pkg/infrastructure/hts"
	"github.com/lanceman/zqk/pkg/objects"
)

func TestDataCell_SignedVerification(t *testing.T) {
	ctx := context.Background()
	signer, _ := crypto.GenerateKeypair()

	// 1. Assemble
	item := map[string]any{objects.FieldKeyID: "SKL-1", objects.FieldKeyKind: objects.KindAgentSkill}
	cell, _ := hts.Assemble(ctx, "CELL-1", []map[string]any{item})

	// 2. Sign
	if err := cell.Sign(signer); err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	// 3. Verify (Success)
	if err := cell.Verify(); err != nil {
		t.Errorf("Verify failed on signed cell: %v", err)
	}

	// 4. Tamper with signature (Failure)
	cell.Signature = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := cell.Verify(); err == nil {
		t.Errorf("expected verification failure on tampered signature")
	}
}
