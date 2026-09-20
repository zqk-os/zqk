package workflow

import (
	"testing"
)

func TestNewCheckCmd(t *testing.T) {
	t.Parallel()
	cmd := NewCheckCmd()
	if cmd == nil {
		t.Fatal("expected command not to be nil")
	}
	if cmd.Use != "check" {
		t.Fatalf("expected use 'check', got %q", cmd.Use)
	}
}
