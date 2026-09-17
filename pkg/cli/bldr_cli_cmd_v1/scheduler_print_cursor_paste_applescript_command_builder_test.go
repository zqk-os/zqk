package bldr_cli_cmd_v1

import "testing"

func TestNewSchedulerPrintCursorPasteApplescriptCommandBuilder(t *testing.T) {
	t.Parallel()
	cmd := NewSchedulerPrintCursorPasteApplescriptCommandBuilder()
	if cmd == nil || cmd.Use != "print-cursor-paste-applescript" {
		t.Fatalf("NewSchedulerPrintCursorPasteApplescriptCommandBuilder Use=%v", cmd)
	}
}
