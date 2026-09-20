package inbox

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
)

func TestNewInboxCmd_HasListSubcommand(t *testing.T) {
	cmd := NewInboxCmd()

	found := false
	for _, sub := range cmd.Commands() {
		if sub.Use == "list" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected 'list' subcommand under inbox")
	}
}

func TestNewInboxCmd_UseName(t *testing.T) {
	cmd := NewInboxCmd()
	if cmd.Use != "inbox" {
		t.Errorf("expected Use='inbox', got %q", cmd.Use)
	}
}

func TestNewListCmd_OutputsTable(t *testing.T) {
	cmd := NewListCmd()
	if cmd.Use != "list" {
		t.Errorf("expected Use='list', got %q", cmd.Use)
	}
}

func executeCmd(root *cobra.Command, args ...string) (string, error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}
