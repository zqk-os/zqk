package storage

import (
	"context"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/objects"
)

type dummyGraphTx struct {
	provider.GraphTransaction
	deleteNodeCalled bool
	deletedID        string
}

func (d *dummyGraphTx) DeleteNode(ctx context.Context, id string, labels []string) error {
	d.deleteNodeCalled = true
	d.deletedID = id
	return nil
}

type dummyStorageReader struct {
	ObjectStorageProvider
}

func (r *dummyStorageReader) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	return map[string]any{
		objects.FieldKeyID:   id,
		objects.FieldKeyKind: "agent_instruction",
	}, nil
}

func TestGraphObjectTransaction_Delete_InTransaction(t *testing.T) {
	mockTx := &dummyGraphTx{}
	reader := &dummyStorageReader{}

	// Instantiate GraphObjectTransaction wrapping reader and mockTx
	tx := &GraphObjectTransaction{
		storage: reader,
		graphTx: mockTx,
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Direct invocation of SUT tx.Delete
	err := tx.Delete(ctx, secCtx, "AGI-sut-test-id", false)
	if err != nil {
		t.Fatalf("tx.Delete failed: %v", err)
	}

	if !mockTx.deleteNodeCalled {
		t.Fatal("expected tx.graphTx.DeleteNode to be called by tx.Delete, but was not called")
	}
	if mockTx.deletedID != "AGI-sut-test-id" {
		t.Fatalf("expected deletedID AGI-sut-test-id, got: %s", mockTx.deletedID)
	}
}
