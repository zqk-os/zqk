package qa

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/policyinterrupt"
)

func TestInterruptEmitter_EmitDisparityInterrupt(t *testing.T) {
	tmpDir := t.TempDir()

	emitter := NewInterruptEmitter(tmpDir)
	err := emitter.EmitDisparityInterrupt(context.Background(), "BLI-123", "Stubbed test logic found.")
	if err != nil {
		t.Fatalf("failed to emit interrupt: %v", err)
	}

	// Verify append via LoadAcksIncremental and LoadLatestCriticalUnacked
	acks, err := policyinterrupt.LoadAcksIncremental(tmpDir)
	if err != nil {
		t.Fatalf("failed to load acks: %v", err)
	}

	latest, err := policyinterrupt.LoadLatestCriticalUnacked(tmpDir, acks)
	if err != nil {
		t.Fatalf("failed to load latest interrupt: %v", err)
	}

	if latest == nil {
		t.Fatal("expected an interrupt record, got nil")
	}

	if latest.DedupeKey != "qa-disparity-BLI-123" {
		t.Errorf("unexpected dedupe key: %s", latest.DedupeKey)
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
