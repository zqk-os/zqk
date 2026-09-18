package meshbroker

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestMCPClientTransport_EnforcesTimeoutAndConcurrency(t *testing.T) {
	// Create a script that sleeps to simulate hanging MCP server
	tmpDir := t.TempDir()
	hangScript := filepath.Join(tmpDir, "hang.sh")
	if err := fileutil.WriteFile(hangScript, []byte("#!/bin/sh\nexec sleep 20\n"), 0755); err != nil {
		t.Fatalf("Failed to write hang script: %v", err)
	}

	transport := NewMCPClientTransport(hangScript)

	ctx := context.Background()
	errCh := make(chan error, 2)
	start := time.Now()

	go func() {
		_, err := transport.getClient(ctx, "endpoint1")
		errCh <- err
	}()

	go func() {
		time.Sleep(100 * time.Millisecond)
		_, err := transport.getClient(ctx, "endpoint2")
		errCh <- err
	}()

	timer := time.NewTimer(12 * time.Second)
	for i := 0; i < 2; i++ {
		select {
		case <-errCh:
		case <-timer.C:
			t.Fatalf("Test timed out! Transport did not enforce its own timeout or deadlocked.")
		}
	}

	duration := time.Since(start)
	if duration > 11*time.Second {
		t.Errorf("Took %v, expected concurrent execution to finish in < 11s", duration)
	}
}
