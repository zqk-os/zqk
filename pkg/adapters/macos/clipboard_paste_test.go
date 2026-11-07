package macos

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/execwrap"
)

func stubCmd(ctx context.Context, _ string, _ ...string) *exec.Cmd {
	return execwrap.CommandContext(ctx, os.Args[0], "-test.run=^$")
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
	var osaArgs []string
	a := &ClipboardPasteAdapter{
		NewCommand: func(ctx context.Context, name string, arg ...string) *exec.Cmd {
			names = append(names, name)
			if name == osascriptBin {
				osaArgs = append([]string{}, arg...)
			}
			return stubCmd(ctx, name, arg...)
		},
	}
	if err := a.Deliver(context.Background(), "hi", "id", nil); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if len(names) != 2 || names[0] != pbcopyBin || names[1] != osascriptBin {
		t.Fatalf("commands=%v", names)
	}
	if len(osaArgs) < 2 || osaArgs[0] != "-e" {
		t.Fatalf("osascript args=%v", osaArgs)
	}
	script := osaArgs[1]
	for _, marker := range []string{`tell application zqkWant to activate`, "refuse", "Terminal", `keystroke "v"`} {
		if !strings.Contains(script, marker) {
			t.Fatalf("paste script missing %q", marker)
		}
	}
}

func TestClipboardPasteAdapter_ResolveTargetApp(t *testing.T) {
	a := &ClipboardPasteAdapter{TargetApp: "CustomIDE"}
	if got := a.resolveTargetApp(nil); got != "CustomIDE" {
		t.Fatalf("struct override: %q", got)
	}
	a = &ClipboardPasteAdapter{}
	if got := a.resolveTargetApp(map[string]string{RenderDetailTargetApp: "FromRender"}); got != "FromRender" {
		t.Fatalf("render detail: %q", got)
	}
	t.Setenv("ZQK_IDE_PASTE_APP", "FromEnv")
	if got := a.resolveTargetApp(nil); got != "FromEnv" {
		t.Fatalf("env: %q", got)
	}
	t.Setenv("ZQK_IDE_PASTE_APP", "")
	if got := a.resolveTargetApp(nil); got != defaultPasteTargetApp {
		t.Fatalf("default: %q", got)
	}
}
