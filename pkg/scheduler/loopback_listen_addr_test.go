package scheduler

import (
	"strings"
	"testing"
)

func TestLoopbackListenAddr_DefaultsToLoopback(t *testing.T) {
	t.Parallel()

	addr := loopbackListenAddr(8080)
	if !strings.HasPrefix(addr, "127.0.0.1:") {
		t.Fatalf("addr = %q, want 127.0.0.1:<port>", addr)
	}
	if strings.HasPrefix(addr, ":") || strings.Contains(addr, "0.0.0.0") {
		t.Fatalf("addr = %q exposes all interfaces", addr)
	}
	if got, want := addr, "127.0.0.1:8080"; got != want {
		t.Fatalf("addr = %q, want %q", got, want)
	}
}
