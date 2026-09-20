package ambient

import (
	"testing"
)

func TestNewAutomergeCmd(t *testing.T) {
	cmd := newAutomergeCmd()
	if cmd == nil {
		t.Fatal("Expected newAutomergeCmd to return a valid command")
	}
	if cmd.Use != "automerge" {
		t.Errorf("Expected command use to be 'automerge', got '%s'", cmd.Use)
	}
}
