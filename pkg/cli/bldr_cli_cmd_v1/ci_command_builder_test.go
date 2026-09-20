package bldr_cli_cmd_v1

import "testing"

func TestNewCiCommandBuilder(t *testing.T) {
	t.Parallel()
	cmd := NewCiCommandBuilder()
	if cmd == nil || cmd.Use != "ci" {
		t.Fatalf("NewCiCommandBuilder Use=%v", cmd)
	}
}
