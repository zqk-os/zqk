package feed

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/mcp"
)

func TestResolveFeedMCPTCP(t *testing.T) {
	t.Parallel()
	if got := resolveFeedMCPTCP(""); got != mcp.DefaultDaemonTCP {
		t.Fatalf("empty=%q", got)
	}
	if got := resolveFeedMCPTCP("  127.0.0.1:9001 "); got != "127.0.0.1:9001" {
		t.Fatalf("got=%q", got)
	}
}
