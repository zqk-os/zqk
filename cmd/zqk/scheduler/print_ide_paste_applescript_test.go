package scheduler

import (
	"slices"
	"testing"
)

func TestNewPrintIDEPasteApplescriptCmd_legacyCursorAlias(t *testing.T) {
	cmd := NewPrintIDEPasteApplescriptCmd()
	if cmd.Use != "print-ide-paste-applescript" {
		t.Fatalf("Use=%q", cmd.Use)
	}
	if !slices.Contains(cmd.Aliases, "print-cursor-paste-applescript") {
		t.Fatalf("Aliases=%v want print-cursor-paste-applescript", cmd.Aliases)
	}
	if cmd.RunE == nil {
		t.Fatal("RunE must be wired (bare builder prints scheduler help; osascript -2740)")
	}
}
