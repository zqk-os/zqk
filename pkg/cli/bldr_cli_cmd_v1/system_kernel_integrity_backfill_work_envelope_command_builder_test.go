package bldr_cli_cmd_v1

import "testing"

func TestNewSystemKernelIntegrityBackfillWorkEnvelopeCommandBuilder(t *testing.T) {
	cmd := NewSystemKernelIntegrityBackfillWorkEnvelopeCommandBuilder()
	if cmd == nil || cmd.Use != "backfill-work-envelope" {
		t.Fatalf("Use=%v", cmd)
	}
	if cmd.Flags().Lookup("apply") == nil || cmd.Flags().Lookup("dry-run") == nil {
		t.Fatal("missing dry-run/apply flags")
	}
}
