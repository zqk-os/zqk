package macos

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/zqk-os/zqk/pkg/execwrap"
)

const (
	// VendorID is the stable adapter name returned by ClipboardPasteAdapter.Vendor.
	VendorID = "macos"

	pbcopyBin    = "pbcopy"
	osascriptBin = "osascript"

	pasteKeystrokeScript = `tell application "System Events" to keystroke "v" using command down`
	pasteDelayScript     = "delay 0.5"
	pasteReturnScript    = `tell application "System Events" to keystroke return`
)

// CommandFactory builds a host command. Tests inject a stub; production uses execwrap.
type CommandFactory func(ctx context.Context, name string, arg ...string) *exec.Cmd

// ClipboardPasteAdapter delivers text by copying it to the macOS clipboard and
// synthesizing Cmd-V + Return through System Events. Darwin-only.
type ClipboardPasteAdapter struct {
	NewCommand CommandFactory
}

// Vendor implements adapters.MessageDelivery.
func (a *ClipboardPasteAdapter) Vendor() string { return VendorID }

// Deliver implements adapters.MessageDelivery.
func (a *ClipboardPasteAdapter) Deliver(ctx context.Context, message string, msgID string, renderDetails map[string]string) error {
	_ = msgID
	_ = renderDetails
	if a.NewCommand == nil && runtime.GOOS != "darwin" {
		return fmt.Errorf("macos clipboard-paste adapter requires darwin (got %s)", runtime.GOOS)
	}

	pbCmd := a.command(ctx, pbcopyBin)
	pbCmd.Stdin = strings.NewReader(message)
	if err := pbCmd.Run(); err != nil {
		return fmt.Errorf("macos pbcopy failed: %w", err)
	}

	osaCmd := a.command(ctx, osascriptBin, "-e", pasteKeystrokeScript, "-e", pasteDelayScript, "-e", pasteReturnScript)
	if err := osaCmd.Run(); err != nil {
		return fmt.Errorf("macos osascript paste failed: %w", err)
	}
	return nil
}

func (a *ClipboardPasteAdapter) command(ctx context.Context, name string, arg ...string) *exec.Cmd {
	if a != nil && a.NewCommand != nil {
		return a.NewCommand(ctx, name, arg...)
	}
	if ctx != nil {
		return execwrap.CommandContext(ctx, name, arg...)
	}
	return execwrap.Command(name, arg...)
}
