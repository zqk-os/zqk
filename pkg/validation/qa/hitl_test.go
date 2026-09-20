package qa

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/policyinterrupt"
	"github.com/zqk-os/zqk/pkg/validation"
)

func TestInterruptEmitter_EmitDisparityInterrupt(t *testing.T) {
	tmpDir := t.TempDir()

	emitter := NewInterruptEmitter(tmpDir)
	err := emitter.EmitDisparityInterrupt(context.Background(), "BLI-123", validation.ConstMagic06bbd9e7)
	if err != nil {
		t.Fatalf(validation.ConstMagic0e62b714, err)
	}

	// Verify append via LoadAcksIncremental and LoadLatestCriticalUnacked
	acks, err := policyinterrupt.LoadAcksIncremental(tmpDir)
	if err != nil {
		t.Fatalf(validation.ConstMagic88f5a720, err)
	}

	latest, err := policyinterrupt.LoadLatestCriticalUnacked(tmpDir, acks)
	if err != nil {
		t.Fatalf(validation.ConstMagic4ed228e0, err)
	}

	if latest == nil {
		t.Fatal(validation.ConstMagicd7b47ee2)
	}

	if latest.DedupeKey != validation.ConstMagic34604fe1 {
		t.Errorf(validation.ConstMagicb47f78ac, latest.DedupeKey)
	}
}

func TestInterruptEmitter_AckDisparityOnPass(t *testing.T) {
	tmpDir := t.TempDir()
	emitter := NewInterruptEmitter(tmpDir)
	if err := emitter.EmitDisparityInterrupt(context.Background(), "BLI-ACK", "missing tests"); err != nil {
		t.Fatal(err)
	}
	acks, err := policyinterrupt.LoadAcksIncremental(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	latest, err := policyinterrupt.LoadLatestCriticalUnacked(tmpDir, acks)
	if err != nil || latest == nil {
		t.Fatalf("expected unacked interrupt, latest=%v err=%v", latest, err)
	}
	emitter.AckDisparityOnPass("BLI-ACK")
	acks, err = policyinterrupt.LoadAcksIncremental(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	latest, err = policyinterrupt.LoadLatestCriticalUnacked(tmpDir, acks)
	if err != nil {
		t.Fatal(err)
	}
	if latest != nil {
		t.Fatalf("expected interrupt cleared after ack-on-pass, got %+v", latest)
	}
}
