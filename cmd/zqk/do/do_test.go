package do

import (
	"testing"
)

func TestNewDoCmd(t *testing.T) {
	cmd := NewDoCmd()
	if cmd == nil {
		t.Fatal("NewDoCmd returned nil")
	}
	if cmd.Use != "do [task_or_bli_id]" {
		t.Errorf("cmd.Use = %q, want do [task_or_bli_id]", cmd.Use)
	}
	if len(cmd.Aliases) == 0 || cmd.Aliases[0] != "auto-exec" {
		t.Errorf("cmd.Aliases = %v, want auto-exec as primary alias", cmd.Aliases)
	}
	if cmd.Flags().Lookup("dry-run") == nil {
		t.Error("expected --dry-run flag")
	}
	if cmd.Flags().Lookup("verify") == nil {
		t.Error("expected --verify flag")
	}
}
