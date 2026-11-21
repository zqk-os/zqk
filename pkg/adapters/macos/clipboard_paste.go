package macos

import (
	"context"
	"fmt"
	"os"
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

	// RenderDetailTargetApp selects which macOS application must be frontmost
	// before keystrokes. Empty falls back to ZQK_IDE_PASTE_APP, then "Cursor".
	RenderDetailTargetApp = "target_app"

	defaultPasteTargetApp = "Cursor"
)

// CommandFactory builds a host command. Tests inject a stub; production uses execwrap.
type CommandFactory func(ctx context.Context, name string, arg ...string) *exec.Cmd

// ClipboardPasteAdapter delivers text by copying it to the macOS clipboard and
// synthesizing Cmd-V + Return through System Events. Darwin-only.
//
// Fail-closed: activates a named target app and refuses to emit keystrokes when
// a terminal (or any other process) is frontmost. Blind frontmost paste mistargets
// Terminal when daemons run paste while the human is in a shell.
type ClipboardPasteAdapter struct {
	NewCommand CommandFactory
	// TargetApp overrides renderDetails / env when non-empty (tests / explicit callers).
	TargetApp string
}

// Vendor implements adapters.MessageDelivery.
func (a *ClipboardPasteAdapter) Vendor() string { return VendorID }

// Deliver implements adapters.MessageDelivery.
func (a *ClipboardPasteAdapter) Deliver(ctx context.Context, message string, msgID string, renderDetails map[string]string) error {
	_ = msgID
	if a.NewCommand == nil && runtime.GOOS != "darwin" {
		return fmt.Errorf("macos clipboard-paste adapter requires darwin (got %s)", runtime.GOOS)
	}

	pbCmd := a.command(ctx, pbcopyBin)
	pbCmd.Stdin = strings.NewReader(message)
	if err := pbCmd.Run(); err != nil {
		return fmt.Errorf("macos pbcopy failed: %w", err)
	}

	target := a.resolveTargetApp(renderDetails)
	script := focusedPasteAppleScript(target)
	osaCmd := a.command(ctx, osascriptBin, "-e", script)
	if err := osaCmd.Run(); err != nil {
		return fmt.Errorf("macos osascript paste failed (target=%s): %w", target, err)
	}
	return nil
}

func (a *ClipboardPasteAdapter) resolveTargetApp(renderDetails map[string]string) string {
	if a != nil {
		if v := strings.TrimSpace(a.TargetApp); v != "" {
			return v
		}
	}
	if renderDetails != nil {
		if v := strings.TrimSpace(renderDetails[RenderDetailTargetApp]); v != "" {
			return v
		}
	}
	if v := strings.TrimSpace(os.Getenv("ZQK_IDE_PASTE_APP")); v != "" {
		return v
	}
	return defaultPasteTargetApp
}

func writeAppleScriptLines(b *strings.Builder, lines ...string) {
	for _, line := range lines {
		b.WriteString(line)
		b.WriteByte('\n')
	}
}

func focusedPasteAppleScript(targetApp string) string {
	app := appleScriptQuoted(targetApp)
	termList := appleScriptQuotedList([]string{
		"Terminal", "iTerm2", "iTerm", "Alacritty", "Warp", "Ghostty", "kitty", "Hyper", "WezTerm",
	})
	var b strings.Builder
	writeAppleScriptLines(&b,
		`set zqkWant to `+app,
		`set zqkGot to false`,
		`repeat 30 times`,
		`	tell application zqkWant to activate`,
		`	delay 0.08`,
		`	tell application "System Events" to set zqkFM to name of first application process whose frontmost is true`,
		`	if zqkFM is zqkWant then`,
		`		set zqkGot to true`,
		`		exit repeat`,
		`	end if`,
		`end repeat`,
		`if zqkGot is false then`,
		`	tell application "System Events" to set zqkFM to name of first application process whose frontmost is true`,
		`	error "macos clipboard-paste: could not frontmost " & zqkWant & " (still " & zqkFM & ")"`,
		`end if`,
		`tell application "System Events"`,
		`	set zqkFM to name of first application process whose frontmost is true`,
		`	if zqkFM is not zqkWant then error "macos clipboard-paste: refuse — frontmost is " & zqkFM & " want " & zqkWant`,
		`	set zqkTermDeny to {`+termList+`}`,
		`	if zqkTermDeny contains zqkFM then error "macos clipboard-paste: refuse terminal mistarget " & zqkFM`,
		`	tell process zqkWant`,
		`		set frontmost to true`,
		`		keystroke "v" using command down`,
		`		delay 0.5`,
		`	end tell`,
		`	set zqkFM to name of first application process whose frontmost is true`,
		`	if zqkFM is not zqkWant then error "macos clipboard-paste: refuse Return — frontmost is " & zqkFM`,
		`	tell process zqkWant`,
		`		set frontmost to true`,
		`		key code 36`,
		`	end tell`,
		`end tell`,
	)
	return b.String()
}

func appleScriptQuoted(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

func appleScriptQuotedList(names []string) string {
	parts := make([]string, 0, len(names))
	for _, n := range names {
		parts = append(parts, appleScriptQuoted(n))
	}
	return strings.Join(parts, ", ")
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
