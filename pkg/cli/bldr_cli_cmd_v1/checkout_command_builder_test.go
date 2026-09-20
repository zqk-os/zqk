package bldr_cli_cmd_v1

import "testing"

func TestNewCheckoutCommandBuilder(t *testing.T) {
	t.Parallel()
	cmd := NewCheckoutCommandBuilder()
	if cmd == nil || cmd.Use != "checkout" {
		t.Fatalf("NewCheckoutCommandBuilder Use=%v", cmd)
	}
	if cmd.Flags().Lookup("sha") == nil {
		t.Fatal("expected --sha flag")
	}
}
