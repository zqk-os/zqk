package system

import (
	"testing"
)

func TestNewStateCommitCmd(t *testing.T) {
	t.Parallel()
	cmd := NewStateCommitCmd()
	if cmd == nil {
		t.Fatal("NewStateCommitCmd() returned nil")
	}

	if cmd.Use != "state-commit" {
		t.Errorf("Expected command use to be 'state-commit', got '%s'", cmd.Use)
	}

	if cmd.Flags().Lookup("snapshot-file") == nil {
		t.Error("Command should have --snapshot-file flag")
	}
}
