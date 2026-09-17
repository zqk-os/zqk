package mcp

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/objects"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// testCtx returns a context for tests; uses system context so handlers see consistent context.
func testCtx() context.Context { return pkgctx.NewSystemContext() }

// TestWorkflowAndCommonToolsReturnWithCancelledContext verifies that when the request
// context is already cancelled, workflow and common tools return quickly (no hang).
// This proves context propagation: the handler uses the request context and
// CommandContext kills the CLI subprocess when ctx is done.
func TestWorkflowAndCommonToolsReturnWithCancelledContext(t *testing.T) {
	t.Parallel()
	dir, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	projectRoot := filepath.Clean(filepath.Join(dir, "..", ".."))

	server := NewServer()
	server.SetProjectRoot(projectRoot)
	server.SetSecurityContext(pkgctx.NewSystemSecurityContext())
	server.SetInitialized(true)

	tools := []struct {
		name string
		args map[string]any
	}{
		{"get_current_backlog_item", map[string]any{objects.FieldKeyFormat: "json"}},
		{"get_next_backlog_item", map[string]any{objects.FieldKeyFormat: "json"}},
		{"get_current_priority_plan", map[string]any{objects.FieldKeyFormat: "json"}},
		{"object_list", map[string]any{objects.FieldKeyKind: "backlog_item", "limit": 1, objects.FieldKeyFormat: "json"}},
		{"object_count", map[string]any{objects.FieldKeyKind: "backlog_item", objects.FieldKeyFormat: "json"}},
		{"system_status", map[string]any{objects.FieldKeyFormat: "json"}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(testCtx())
			cancel() // cancel immediately so handler should return quickly (subprocess killed or not started)

			done := make(chan struct{})
			var callErr error
			goroutinelabels.NewGoroutine("mcp_test", "tool call with cancelled context").StartSimple(func() {
				_, callErr = server.handleToolCallWithContext(ctx, tt.name, tt.args)
				close(done)
			})

			select {
			case <-done:
				// Handler returned; we expect context.Canceled or similar when ctx was already cancelled
				if callErr != nil && !errors.Is(callErr, context.Canceled) && ctx.Err() != context.Canceled {
					t.Logf("tool returned with error (expected when ctx cancelled): %v", callErr)
				}
			case <-time.After(10 * time.Second):
				t.Fatalf("tool %s did not return within 10s with already-cancelled context (hang)", tt.name)
			}
		})
	}
}

// TestWorkflowToolReturnsWithinRequestTimeout verifies one tool (get_current_backlog_item)
// returns within the request timeout. Uses a 5s context; if the CLI runs longer we rely on
// context cancellation to kill the subprocess and return. Max wait 8s.
func TestWorkflowToolReturnsWithinRequestTimeout(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping CLI-invoking test in short mode")
	}
	projectRoot := t.TempDir()

	server := NewServer()
	server.SetProjectRoot(projectRoot)
	server.SetSecurityContext(pkgctx.NewSystemSecurityContext())
	server.SetInitialized(true)

	ctx, cancel := context.WithTimeout(testCtx(), 5*time.Second)
	defer cancel()

	done := make(chan struct{})
	goroutinelabels.NewGoroutine("mcp_test", "tool call with timeout").StartSimple(func() {
		_, _ = server.handleToolCallWithContext(ctx, "get_current_backlog_item", map[string]any{objects.FieldKeyFormat: "json"})
		close(done)
	})

	select {
	case <-done:
		// Returned within 8s (either success, error, or context cancelled at 5s)
	case <-time.After(8 * time.Second):
		t.Fatalf("get_current_backlog_item did not return within 8s (request timeout 5s); possible hang")
	}
}

// TestClientCancellationAbortsToolCall verifies that sending a notifications/cancelled
// payload aborts an in-flight long-running tool call, ensuring compliance with the MCP spec.
func TestClientCancellationAbortsToolCall(t *testing.T) {
	t.Parallel()
	server := NewServer()
	server.SetInitialized(true)

	started := make(chan struct{})
	finished := make(chan struct{})

	// Register a slow tool that we can cancel
	server.RegisterTool(
		"long_running_tool",
		"A long running tool",
		nil,
		func(ctx context.Context, args map[string]any) (any, error) {
			close(started)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(10 * time.Second):
				close(finished)
				return nil, fmt.Errorf("timeout without cancellation")
			}
		},
	)

	ctx, cancel := context.WithTimeout(testCtx(), 5*time.Second)
	defer cancel()

	// We simulate message processor tracking manually
	opCtx, opCancel := server.operationTracker.StartOperation("test-req-id", "tools/call", ctx)
	defer opCancel()

	done := make(chan struct{})
	var callErr error
	goroutinelabels.NewGoroutine("mcp_test", "tool call cancellation").StartSimple(func() {
		_, callErr = server.handleToolCallWithContext(opCtx, "long_running_tool", map[string]any{})
		close(done)
	})

	// Wait for tool to start
	<-started

	// Simulate cancellation from client
	cancelParams := []byte(`{"requestId": "test-req-id", "reason": "client requested cancel"}`)
	_, _ = server.handleNotificationCancelled(context.Background(), "notifications/cancelled", cancelParams)

	select {
	case <-done:
		if callErr == nil || !errors.Is(callErr, context.Canceled) {
			t.Errorf("expected context.Canceled error, got: %v", callErr)
		}
	case <-finished:
		t.Errorf("tool finished without being cancelled")
	case <-time.After(2 * time.Second):
		t.Errorf("tool did not return promptly after cancellation")
	}
}
