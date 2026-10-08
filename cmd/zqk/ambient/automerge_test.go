package ambient

import (
	"context"
	"testing"
)

func TestNewAutomergeCmd(t *testing.T) {
	cmd := newAutomergeCmd()
	if cmd == nil {
		t.Fatal("Expected newAutomergeCmd to return a valid command")
	}
	if cmd.Use != "automerge" {
		t.Errorf("Expected command use to be 'automerge', got '%s'", cmd.Use)
	}
}

func TestRunAmbientAutomerge_ContextCancelled(t *testing.T) {
	cmd := newAutomergeCmd()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd.SetContext(ctx)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected nil error from cancelled automerge cmd, got: %v", err)
	}
}
