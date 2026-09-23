package agentclaim

import (
	"context"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestReclaimOrMintFallback_skipsWhenRealATKFree(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_ = objects.GetGlobalLifecycleLoader().EnsureReady(ctx)
	sec := pkgctx.NewSystemSecurityContext()
	store := newClaimMemStore(map[string]any{
		objects.FieldKeyID:     "ATK-real",
		objects.FieldKeyKind:   objects.KindAgentTask,
		objects.FieldKeyStatus: objects.ObjectStatusApproved,
		objects.FieldKeyTitle:  "real work",
	})
	id, minted, err := ReclaimOrMintFallback(ctx, store, sec, "seat-a", t.TempDir())
	if err != nil {
		t.Fatalf("ReclaimOrMintFallback: %v", err)
	}
	if id != "" || minted {
		t.Fatalf("pool has a real ATK; must not mint fallback: id=%q minted=%v", id, minted)
	}
}

func TestReclaimOrMintFallback_mintsWhenPoolEmpty(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_ = objects.GetGlobalLifecycleLoader().EnsureReady(ctx)
	sec := pkgctx.NewSystemSecurityContext()
	store := newClaimMemStore()
	root := t.TempDir()
	id, minted, err := ReclaimOrMintFallback(ctx, store, sec, "seat-b", root)
	if err != nil {
		t.Fatalf("ReclaimOrMintFallback: %v", err)
	}
	if !minted || !strings.HasPrefix(strings.ToUpper(id), "ATK-") {
		t.Fatalf("empty pool must mint an ATK: id=%q minted=%v", id, minted)
	}
	obj, err := store.Read(ctx, sec, id)
	if err != nil {
		t.Fatalf("read minted: %v", err)
	}
	if !IsFallbackOccupancy(obj) {
		t.Fatalf("minted object must be marked fallback: %#v", obj)
	}
	if objects.StringField(obj, objects.FieldKeyClaimedBy) != "seat-b" {
		t.Fatalf("minted fallback must be claimed: %#v", obj)
	}

	id2, minted2, err := ReclaimOrMintFallback(ctx, store, sec, "seat-b", root)
	if err != nil {
		t.Fatalf("reclaim: %v", err)
	}
	if minted2 || id2 != id {
		t.Fatalf("second call must reclaim %s, minted=%v got %s", id, minted2, id2)
	}
}

func TestGateWrite_emptyPoolFallbackInterrupts(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dec, err := GateWrite(root, "seat-c", GateOptions{
		EmptyPoolFallback: func() (string, bool, error) {
			return "ATK-fallback-1", true, nil
		},
	})
	if err != nil {
		t.Fatalf("GateWrite: %v", err)
	}
	if dec.Allowed || dec.Reason != ReasonFallbackMinted || dec.AutoAssignedID != "ATK-fallback-1" {
		t.Fatalf("empty-pool must interrupt onto minted ATK: %+v", dec)
	}
	if !strings.Contains(dec.Message, "ATK-fallback-1") {
		t.Fatalf("interrupt must name the minted task: %q", dec.Message)
	}
}
