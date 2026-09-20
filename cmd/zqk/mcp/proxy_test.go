package mcp

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestProxyCmd_RunProxy(t *testing.T) {
	cmd := NewProxyCmd()
	cmd.SetArgs([]string{"--tcp", "127.0.0.1:0"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd.SetContext(ctx)

	// Create dummy pipe to avoid reading from real stdin which blocks
	r, _, _ := os.Pipe()
	defer r.Close()
	os.Stdin = r

	errCh := make(chan error, 1)
	go func() {
		errCh <- cmd.Execute()
	}()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("runProxy failed: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		cancel()
	}
}
