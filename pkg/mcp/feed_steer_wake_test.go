package mcp

import "testing"

func TestResolveDaemonTCP(t *testing.T) {
	t.Parallel()
	if got := ResolveDaemonTCP(""); got != DefaultDaemonTCP {
		t.Fatalf("empty=%q", got)
	}
	if got := ResolveDaemonTCP("  127.0.0.1:9001 "); got != "127.0.0.1:9001" {
		t.Fatalf("got=%q", got)
	}
}
