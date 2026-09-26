package macos

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"testing"
)

func stubCmd(ctx context.Context, _ string, _ ...string) *exec.Cmd {
	return exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
}

func TestClipboardPasteAdapter_Vendor(t *testing.T) {
	a := &ClipboardPasteAdapter{}
	if a.Vendor() != VendorID {
		t.Fatalf("Vendor=%q want %q", a.Vendor(), VendorID)
	}
}

func TestClipboardPasteAdapter_FailsClosedOffDarwin(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("host check only fails off darwin")
	}
	err := (&ClipboardPasteAdapter{}).Deliver(context.Background(), "hi", "id", nil)
	if err == nil {
		t.Fatal("expected darwin fail-closed error")
	}
}

func TestClipboardPasteAdapter_InvokesPbcopyAndOsascript(t *testing.T) {
	var names []string
	a := &ClipboardPasteAdapter{
		NewCommand: func(ctx context.Context, name string, arg ...string) *exec.Cmd {
			names = append(names, name)
			return stubCmd(ctx, name, arg...)
		},
	}
	if err := a.Deliver(context.Background(), "hi", "id", nil); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if len(names) != 2 || names[0] != pbcopyBin || names[1] != osascriptBin {
		t.Fatalf("commands=%v", names)
	}
}
