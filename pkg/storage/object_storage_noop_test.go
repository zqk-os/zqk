package storage

import (
	"context"
	"errors"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

func TestNewNoopObjectStorage_CreateOk_OtherOpsErr(t *testing.T) {

	sp := NewNoopObjectStorage()
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	if err := sp.Create(ctx, secCtx, map[string]any{}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := sp.Read(ctx, secCtx, "any"); err == nil || !errors.Is(err, ErrNoopObjectStorage) {
		t.Fatalf("Read: want %v, got %v", ErrNoopObjectStorage, err)
	}
}
