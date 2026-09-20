package bldr_cli_cmd_v1

import "testing"

func TestNewMcpSvcCursorAdapterCommandBuilder(t *testing.T) {
	t.Parallel()
	cmd := NewMcpSvcCursorAdapterCommandBuilder()
	if cmd == nil || cmd.Use != "cursor-adapter" {
		t.Fatalf("NewMcpSvcCursorAdapterCommandBuilder Use=%v", cmd)
	}
}
