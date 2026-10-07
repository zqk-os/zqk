package cli

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
)

func TestCommandSignalContext(t *testing.T) {
	// Command without context
	cmd := &cobra.Command{Use: "test"}
	ctx, cancel := CommandSignalContext(cmd)
	if ctx == nil || cancel == nil {
		t.Fatalf("expected non-nil context and cancel func")
	}
	cancel()

	// Command with existing context
	parentCtx, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()
	cmd.SetContext(parentCtx)

	ctx2, cancel2 := CommandSignalContext(cmd)
	if ctx2 == nil || cancel2 == nil {
		t.Fatalf("expected non-nil context and cancel func with parent")
	}
	cancel2()
}
