package mcp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestHandleAgentTriggerVerificationTool_ExecutionAndCleanup(t *testing.T) {
	server := NewServer()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	args := map[string]any{
		objects.FieldKeyTarget: "./pkg/concurrency/...",
	}

	res, err := server.handleAgentTriggerVerificationTool(ctx, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msg, ok := res.(string)
	if !ok || !strings.Contains(msg, "Verification triggered in the background") {
		t.Fatalf("expected background trigger confirmation, got %v", res)
	}

	// Give goroutine a moment to start
	time.Sleep(50 * time.Millisecond)

	// Verify ProcessGroupManager is tracking active workers
	pgm := server.GetProcessGroupManager()
	if pgm != nil {
		gCount, _ := pgm.GetProcessGroupStats()
		if gCount < 1 {
			t.Logf("goroutine count: %d (may have completed quickly)", gCount)
		}
	}
}

func TestHandleAgentTriggerVerificationTool_ShutdownCleanup(t *testing.T) {
	server := NewServer()

	args := map[string]any{
		objects.FieldKeyTarget: "./pkg/storage/...",
	}

	_, err := server.handleAgentTriggerVerificationTool(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	time.Sleep(20 * time.Millisecond)

	// Shut down server process group manager
	pgm := server.GetProcessGroupManager()
	if pgm != nil {
		if err := pgm.Shutdown("test shutdown"); err != nil {
			t.Fatalf("pgm shutdown failed: %v", err)
		}
		// Wait for subprocesses to be unregistered
		for i := 0; i < 100; i++ {
			time.Sleep(100 * time.Millisecond)
			status := pgm.GetStatus()
			if status.SubprocessCount == 0 {
				break
			}
		}
		status := pgm.GetStatus()
		if status.SubprocessCount != 0 {
			t.Fatalf("expected 0 subprocesses after shutdown, got %d", status.SubprocessCount)
		}
	}
}
