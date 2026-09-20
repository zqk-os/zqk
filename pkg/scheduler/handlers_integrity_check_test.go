package scheduler

import (
	"context"
	"testing"
)

func TestIntegrityCheckHandler_Creation(t *testing.T) {
	handler := NewIntegrityCheckHandler(nil)
	if handler == nil {
		t.Fatal("expected non-nil IntegrityCheckHandler")
	}

	args := integrityCheckCommandArgs()
	if len(args) == 0 {
		t.Fatal("expected non-empty args")
	}
	if args[0] != "system" || args[1] != "check" {
		t.Fatalf("unexpected args: %v", args)
	}

	// Test execute with nil job returns error
	ctx := context.Background()
	err := handler.Execute(ctx, nil)
	if err == nil {
		t.Fatal("expected error for nil job")
	}
}
