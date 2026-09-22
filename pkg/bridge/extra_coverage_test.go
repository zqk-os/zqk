// BLI-STARTER-COMMUNITY-051 / PRI-STARTER-COMMUNITY-051 coverage elevation
package bridge

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/infrastructure"
	"github.com/zqk-os/zqk/pkg/infrastructure/crypto"
	"github.com/zqk-os/zqk/pkg/infrastructure/hts"
	"github.com/zqk-os/zqk/pkg/objects"
)

type extraSpine struct {
	last infrastructure.Event
}

func (s *extraSpine) Publish(_ context.Context, event infrastructure.Event) error {
	s.last = event
	return nil
}
func (s *extraSpine) Subscribe(context.Context, string, infrastructure.Handler) error { return nil }
func (s *extraSpine) Replay(context.Context, int64, infrastructure.Handler) error     { return nil }
func (s *extraSpine) Close() error                                                    { return nil }

func TestExtraP2PTransferAndConflict(t *testing.T) {
	ctx := context.Background()
	cell, err := hts.Assemble(ctx, "CELL-1", []map[string]any{
		{objects.FieldKeyID: "OBJ-1", objects.FieldKeyKind: "note", "body": "x"},
	})
	if err != nil {
		t.Fatal(err)
	}
	signer, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	spine := &extraSpine{}
	p := NewP2PTransferProtocol(spine, signer)
	if err := p.Transfer(ctx, "src", "dst", cell); err != nil {
		t.Fatal(err)
	}
	if err := p.HandleTransfer(ctx, spine.last); err != nil {
		t.Fatal(err)
	}
	if err := p.HandleTransfer(ctx, infrastructure.Event{Payload: map[string]any{}}); err == nil {
		t.Fatal("missing tde")
	}
	if err := p.HandleTransfer(ctx, infrastructure.Event{Payload: map[string]any{"tde_envelope": "nope"}}); err == nil {
		t.Fatal("bad tde")
	}

	unsigned := NewP2PTransferProtocol(spine, nil)
	cell2, err := hts.Assemble(ctx, "CELL-2", []map[string]any{
		{objects.FieldKeyID: "OBJ-2", objects.FieldKeyKind: "note"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := unsigned.Transfer(ctx, "src", "dst", cell2); err != nil {
		t.Fatal(err)
	}
	if err := unsigned.HandleTransfer(ctx, spine.last); err == nil {
		t.Fatal("unsigned tde")
	}

	bad := *cell
	bad.RootHash = "deadbeef"
	if err := NewP2PTransferProtocol(spine, nil).Transfer(ctx, "src", "dst", &bad); err == nil {
		t.Fatal("invalid cell")
	}

	vcA := VectorClock{"a": 1, "b": 0}
	vcB := VectorClock{"a": 0, "b": 1}
	cr := NewConflictResolver("a")
	_, _, _, err = cr.ResolveState(map[string]any{"k": "aa"}, vcA, map[string]any{"k": "zz", "n": 1}, vcB)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, _ = cr.ResolveState(map[string]any{"k": 1}, VectorClock{"a": 1}, map[string]any{"k": 2}, VectorClock{"a": 2})
	_, _, _, _ = cr.ResolveState(map[string]any{"k": 2}, VectorClock{"a": 2}, map[string]any{"k": 1}, VectorClock{"a": 1})
	_ = vcA.Clone()
	_ = vcA.Merge(vcB)
	vcA.Increment("a")
}
