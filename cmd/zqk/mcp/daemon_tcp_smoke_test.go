package mcp

import (
	"testing"
)

// TestDaemonCommandWiresTCPFlag ensures mcp daemon exposes the TCP listen flag
// expected by the stdio proxy (default 127.0.0.1:8443).
func TestDaemonCommandWiresTCPFlag(t *testing.T) {
	cmd := NewDaemonCmd()
	f := cmd.Flags().Lookup("tcp")
	if f == nil {
		t.Fatal("expected --tcp flag on mcp daemon")
	}
	if f.DefValue != DefaultMCPDaemonTCP {
		t.Fatalf("unexpected --tcp default: %q", f.DefValue)
	}
	if cmd.Flags().Lookup("port") == nil {
		t.Fatal("expected --port flag on mcp daemon")
	}
}
