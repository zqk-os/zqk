package execwrap

import (
	"context"
	"testing"
)

func TestCommand(t *testing.T) {
	cmd := Command("echo", "test")
	if cmd.Args[0] != "echo" {
		t.Fatal("expected echo")
	}
}

func TestCommandContext(t *testing.T) {
	cmd := CommandContext(context.Background(), "echo", "test")
	if cmd.Args[0] != "echo" {
		t.Fatal("expected echo")
	}
}
