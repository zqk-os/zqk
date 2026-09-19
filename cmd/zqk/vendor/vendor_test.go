package vendor

import (
	"testing"
)

func TestVendorCommandHierarchy(t *testing.T) {
	cmd := NewVendorCmd()
	if cmd.Use != "vendor" {
		t.Fatalf("expected Use 'vendor', got %q", cmd.Use)
	}

	cursorCmd, _, err := cmd.Find([]string{"cursor"})
	if err != nil || cursorCmd == nil {
		t.Fatalf("expected 'cursor' subcommand under vendor, got err=%v", err)
	}

	adapterCmd, _, err := cursorCmd.Find([]string{"adapter"})
	if err != nil || adapterCmd == nil {
		t.Fatalf("expected 'adapter' subcommand under vendor cursor, got err=%v", err)
	}

	pasteCmd, _, err := cursorCmd.Find([]string{"paste-applescript"})
	if err != nil || pasteCmd == nil {
		t.Fatalf("expected 'paste-applescript' subcommand under vendor cursor, got err=%v", err)
	}
}
