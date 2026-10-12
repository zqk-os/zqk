package router

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"time"
)

func TestGradientRouter_BuilderAndRouting(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	gr := NewGradientRouterBuilder().
		WithFastTimeout(10 * time.Second).
		WithStandardTimeout(30 * time.Second).
		WithHeavyTimeout(60 * time.Second).
		WithLocalTimeout(90 * time.Second).
		WithWorkspaceRoot(tmpDir).
		WithDefaultModelHandler(func(ctx context.Context, req *ModelRequest) (*ModelResponse, error) {
			return &ModelResponse{
				Content:    "default-model-content",
				TokensUsed: 42,
			}, nil
		}).
		WithDefaultToolHandler(func(ctx context.Context, call *ToolCall) (*ToolResult, error) {
			return &ToolResult{
				ToolName: call.ToolName,
				Output:   "default-tool-output",
			}, nil
		}).
		Build()

	// 1. Model routing
	mResp, err := gr.RouteModel(context.Background(), &ModelRequest{
		Model: "gemini-2.5-flash",
	})
	if err != nil {
		t.Fatalf("RouteModel failed: %v", err)
	}
	if mResp.Tier != TierFast {
		t.Errorf("expected TierFast, got %q", mResp.Tier)
	}

	// 2. In-scope tool routing
	tRes, err := gr.RouteTool(context.Background(), &ToolCall{
		ToolName: ToolReadFile,
		Arguments: map[string]any{
			ArgPath: filepath.Join(tmpDir, "file.txt"),
		},
	})
	if err != nil {
		t.Fatalf("RouteTool failed: %v", err)
	}
	if !tRes.Passthrough {
		t.Errorf("expected Passthrough=true on default tool handler")
	}

	// 3. Out-of-scope tool routing (intercepted)
	_, err = gr.RouteTool(context.Background(), &ToolCall{
		ToolName: ToolReadFile,
		Arguments: map[string]any{
			ArgPath: filepath.Join(string(filepath.Separator), "forbidden", "path"),
		},
	})
	if err == nil {
		t.Fatalf("expected out of scope error")
	}

	// 4. Verify telemetry stats
	stats := gr.Stats()
	if stats.ModelRequestsTotal != 1 {
		t.Errorf("expected ModelRequestsTotal=1, got %d", stats.ModelRequestsTotal)
	}
	if stats.ModelRequestsByTier[TierFast] != 1 {
		t.Errorf("expected TierFast count=1, got %d", stats.ModelRequestsByTier[TierFast])
	}
	if stats.ToolCallsTotal != 2 {
		t.Errorf("expected ToolCallsTotal=2, got %d", stats.ToolCallsTotal)
	}
	if stats.ToolCallsIntercepted != 1 {
		t.Errorf("expected ToolCallsIntercepted=1, got %d", stats.ToolCallsIntercepted)
	}
	if stats.ToolCallsPassthrough != 1 {
		t.Errorf("expected ToolCallsPassthrough=1, got %d", stats.ToolCallsPassthrough)
	}
}

func TestGradientRouter_ConcurrentStress(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	gr := NewGradientRouterBuilder().
		WithWorkspaceRoot(tmpDir).
		WithDefaultModelHandler(func(ctx context.Context, req *ModelRequest) (*ModelResponse, error) {
			return &ModelResponse{
				Content:    fmt.Sprintf("resp-%s", req.Model),
				TokensUsed: 10,
			}, nil
		}).
		WithDefaultToolHandler(func(ctx context.Context, call *ToolCall) (*ToolResult, error) {
			return &ToolResult{
				ToolName: call.ToolName,
				Output:   "ok",
			}, nil
		}).
		Build()

	var wg sync.WaitGroup
	const workers = 16
	const iterations = 50

	for i := 0; i < workers; i++ {
		wg.Add(1)
		workerID := i
		goroutinelabels.NewGoroutine("gradient_router_worker", "stress_test").StartSimple(func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				// Concurrent model requests
				model := "gemini-2.5-flash"
				if j%2 == 0 {
					model = "claude-3-5-sonnet"
				} else if j%3 == 0 {
					model = "ollama/llama3"
				}
				_, _ = gr.RouteModel(context.Background(), &ModelRequest{Model: model})

				// Concurrent tool calls (alternating valid and invalid paths)
				p := filepath.Join(tmpDir, fmt.Sprintf("file-%d-%d.txt", workerID, j))
				if j%5 == 0 {
					p = filepath.Join(string(filepath.Separator), "invalid", "path.txt")
				}
				_, _ = gr.RouteTool(context.Background(), &ToolCall{
					ToolName: ToolReadFile,
					Arguments: map[string]any{
						ArgPath: p,
					},
				})
			}
		})
	}

	wg.Wait()

	stats := gr.Stats()
	if stats.ModelRequestsTotal != uint64(workers*iterations) {
		t.Errorf("expected %d model requests, got %d", workers*iterations, stats.ModelRequestsTotal)
	}
	if stats.ToolCallsTotal != uint64(workers*iterations) {
		t.Errorf("expected %d tool calls, got %d", workers*iterations, stats.ToolCallsTotal)
	}
}

func TestGradientRouter_RegistrationMethods(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	extraRoot := t.TempDir()

	gr := NewGradientRouterBuilder().
		WithWorkspaceRoot(tmpDir).
		WithAllowedRoots(extraRoot).
		WithScopeMode(ScopeModeStrict).
		Build()

	// Test RegisterModel
	gr.RegisterModel("custom-fast", func(ctx context.Context, req *ModelRequest) (*ModelResponse, error) {
		return &ModelResponse{Content: "custom-fast-res"}, nil
	})

	// Test RegisterTier
	gr.RegisterTier(TierLocal, func(ctx context.Context, req *ModelRequest) (*ModelResponse, error) {
		return &ModelResponse{Content: "local-tier-res"}, nil
	})

	// Test RegisterTool
	gr.RegisterTool("custom_tool", func(ctx context.Context, call *ToolCall) (*ToolResult, error) {
		return &ToolResult{Output: "custom_tool_res"}, nil
	})

	// Test AddToolInterceptor
	interceptorCalled := false
	gr.AddToolInterceptor(func(ctx context.Context, call *ToolCall, next ToolHandler) (*ToolResult, error) {
		interceptorCalled = true
		return next(ctx, call)
	})

	// 1. Verify custom model routing
	mResp, err := gr.RouteModel(context.Background(), &ModelRequest{Model: "custom-fast"})
	if err != nil || mResp.Content != "custom-fast-res" {
		t.Fatalf("expected custom model response, got %v, err: %v", mResp, err)
	}

	// 2. Verify tier routing
	mRespTier, err := gr.RouteModel(context.Background(), &ModelRequest{Model: "ollama/llama3"})
	if err != nil || mRespTier.Content != "local-tier-res" {
		t.Fatalf("expected local tier response, got %v, err: %v", mRespTier, err)
	}

	// 3. Verify custom tool routing with interceptor
	tRes, err := gr.RouteTool(context.Background(), &ToolCall{ToolName: "custom_tool"})
	if err != nil || tRes.Output != "custom_tool_res" {
		t.Fatalf("expected custom tool response, got %v, err: %v", tRes, err)
	}
	if !interceptorCalled {
		t.Fatalf("expected interceptor to be called")
	}

	// 4. Verify allowed extra root
	allowedPath := filepath.Join(extraRoot, "extra.txt")
	gr.RegisterTool(ToolReadFile, func(ctx context.Context, call *ToolCall) (*ToolResult, error) {
		return &ToolResult{Output: "extra ok"}, nil
	})
	tResAllowed, err := gr.RouteTool(context.Background(), &ToolCall{
		ToolName: ToolReadFile,
		Arguments: map[string]any{
			ArgPath: allowedPath,
		},
	})
	if err != nil || tResAllowed.Output != "extra ok" {
		t.Fatalf("expected path in extra allowed root to succeed, got %v, err: %v", tResAllowed, err)
	}
}
