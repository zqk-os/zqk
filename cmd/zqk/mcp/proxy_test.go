package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestProxyCmd_RunProxy(t *testing.T) {
	cmd := NewProxyCmd()
	cmd.SetArgs([]string{"--tcp", "127.0.0.1:0"})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	cmd.SetContext(ctx)

	errCh := make(chan error, 1)
	goroutinelabels.StartNamedGoroutine("mcp_test_proxy", "run mcp proxy command", func() {
		errCh <- cmd.Execute()
	})

	select {
	case err := <-errCh:
		if err != nil && err != context.DeadlineExceeded && err != context.Canceled {
			t.Fatalf("runProxy failed: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		cancel()
	}
}
