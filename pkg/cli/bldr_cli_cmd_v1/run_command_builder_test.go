package bldr_cli_cmd_v1

import "testing"

func TestNewRunCommandBuilder(t *testing.T) {
	t.Parallel()
	cmd := NewRunCommandBuilder()
	if cmd == nil || cmd.Use != "run" {
		t.Fatalf("NewRunCommandBuilder Use=%v", cmd)
	}
	if cmd.Flags().Lookup("package") == nil {
		t.Fatal("expected --package flag")
	}
}
