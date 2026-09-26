// BLI-STARTER-COMMUNITY-028 / PRI-STARTER-COMMUNITY-028 coverage elevation
package macos

import (
	"context"
	"os/exec"
	"testing"
)

func failCmd(ctx context.Context, _ string, _ ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "false")
}

func TestClipboardPasteAdapter_PbcopyFailure(t *testing.T) {
	t.Parallel()
	a := &ClipboardPasteAdapter{NewCommand: failCmd}
	if err := a.Deliver(context.Background(), "hi", "id", nil); err == nil {
		t.Fatal("expected pbcopy failure")
	}
}

func TestClipboardPasteAdapter_OsascriptFailure(t *testing.T) {
	t.Parallel()
	n := 0
	a := &ClipboardPasteAdapter{
		NewCommand: func(ctx context.Context, name string, arg ...string) *exec.Cmd {
			n++
			if name == pbcopyBin {
				return exec.CommandContext(ctx, "true")
			}
			return exec.CommandContext(ctx, "false")
		},
	}
	if err := a.Deliver(context.Background(), "hi", "id", nil); err == nil {
		t.Fatal("expected osascript failure")
	}
	if n != 2 {
		t.Fatalf("calls=%d", n)
	}
}

func TestClipboardPasteAdapter_commandDefault(t *testing.T) {
	t.Parallel()
	a := &ClipboardPasteAdapter{}
	if cmd := a.command(context.Background(), "true"); cmd == nil {
		t.Fatal("context command")
	}
	if cmd := a.command(nil, "true"); cmd == nil {
		t.Fatal("no-context command")
	}
	var n *ClipboardPasteAdapter
	if cmd := n.command(context.Background(), "true"); cmd == nil {
		t.Fatal("nil receiver still builds a command")
	}
}

func TestClipboardPasteAdapter_commandUsesFactory(t *testing.T) {
	t.Parallel()
	called := false
	a := &ClipboardPasteAdapter{
		NewCommand: func(ctx context.Context, name string, arg ...string) *exec.Cmd {
			called = true
			return exec.CommandContext(ctx, "true")
		},
	}
	cmd := a.command(context.Background(), "true")
	if !called || cmd == nil {
		t.Fatal("factory")
	}
}
