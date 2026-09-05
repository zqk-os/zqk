package bldr_cli_cmd_v1

import "testing"

func TestNewKernelIntegrityCommandBuilder(t *testing.T) {
	cmd := NewKernelIntegrityCommandBuilder()
	if cmd == nil || cmd.Use != "kernel-integrity" {
		t.Fatalf("NewKernelIntegrityCommandBuilder Use=%v", cmd)
	}
	compose := NewSystemKernelIntegrityComposeCommandBuilder()
	if compose == nil || compose.Flags().Lookup("dry-run") == nil {
		t.Fatal("compose builder missing --dry-run")
	}
	heal := NewSystemKernelIntegrityHealDanglingCommandBuilder()
	if heal == nil || heal.Flags().Lookup("apply") == nil || heal.Flags().Lookup("limit") == nil {
		t.Fatal("heal-dangling builder missing flags")
	}
	backfill := NewSystemKernelIntegrityBackfillWorkEnvelopeCommandBuilder()
	if backfill == nil || backfill.Flags().Lookup("apply") == nil || backfill.Flags().Lookup("limit") == nil {
		t.Fatal("backfill-work-envelope builder missing flags")
	}
}
