package router

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
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

func TestGradientRouter_Route_InRepoQuery(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	gr := NewGradientRouter(nil, NewToolScope(tmpDir))

	req := &RouteRequest{
		CommandLine: "grep -rn \"pattern\" pkg/",
		ProjectRoot: tmpDir,
		TokenBudget: 2000,
	}

	decision, err := gr.Route(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected route error: %v", err)
	}
	if !decision.Intercepted {
		t.Errorf("expected Intercepted=true, got false")
	}
	if decision.Fallback {
		t.Errorf("expected Fallback=false, got true")
	}
	if decision.Action != ActionIntercept {
		t.Errorf("expected Action=%s, got %s", ActionIntercept, decision.Action)
	}
	if decision.TargetTool != "zqk grep" {
		t.Errorf("expected TargetTool='zqk grep', got %s", decision.TargetTool)
	}
	if !strings.Contains(decision.RewrittenCommand, "zqk grep") {
		t.Errorf("expected RewrittenCommand to contain 'zqk grep', got %q", decision.RewrittenCommand)
	}
	if !strings.Contains(decision.RewrittenCommand, "--max-tokens") {
		t.Errorf("expected RewrittenCommand to contain '--max-tokens', got %q", decision.RewrittenCommand)
	}
}

func TestGradientRouter_Route_ExternalPathBypass(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	gr := NewGradientRouter(nil, NewToolScope(tmpDir))

	req := &RouteRequest{
		CommandLine: "grep \"foo\" /tmp/outside.log",
		ProjectRoot: tmpDir,
	}

	decision, err := gr.Route(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected route error: %v", err)
	}
	if decision.Intercepted {
		t.Errorf("expected Intercepted=false for external path")
	}
	if !decision.Fallback {
		t.Errorf("expected Fallback=true for external path")
	}
	if !decision.Bypassed {
		t.Errorf("expected Bypassed=true for external path")
	}
	if decision.Reason != ReasonExternalPath {
		t.Errorf("expected Reason=%s, got %s", ReasonExternalPath, decision.Reason)
	}
	if decision.Stage != StageInRepoFilter {
		t.Errorf("expected Stage=%s, got %s", StageInRepoFilter, decision.Stage)
	}
	if decision.RewrittenCommand != req.CommandLine {
		t.Errorf("expected RewrittenCommand unchanged, got %q", decision.RewrittenCommand)
	}
}

func TestGradientRouter_Route_UnsupportedFlags(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	gr := NewGradientRouter(nil, NewToolScope(tmpDir))

	req := &RouteRequest{
		CommandLine: "grep -v \"pattern\" pkg/",
		ProjectRoot: tmpDir,
	}

	decision, err := gr.Route(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected route error: %v", err)
	}
	if decision.Intercepted {
		t.Errorf("expected Intercepted=false for unsupported flags")
	}
	if !decision.Fallback {
		t.Errorf("expected Fallback=true for unsupported flags")
	}
	if !decision.Bypassed {
		t.Errorf("expected Bypassed=true for unsupported flags")
	}
	if decision.Reason != ReasonUnsupportedFlags {
		t.Errorf("expected Reason=%s, got %s", ReasonUnsupportedFlags, decision.Reason)
	}
	if decision.Stage != StageFunnelFilter {
		t.Errorf("expected Stage=%s, got %s", StageFunnelFilter, decision.Stage)
	}
}

func TestGradientRouter_Route_TokenBudgetThresholds(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	gr := NewGradientRouter(nil, NewToolScope(tmpDir))

	// Explicit 500 tokens
	req500 := &RouteRequest{
		CommandLine: "grep -r \"func\" pkg/",
		ProjectRoot: tmpDir,
		TokenBudget: 500,
	}
	dec500, err := gr.Route(context.Background(), req500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(dec500.RewrittenCommand, "--max-tokens 500") {
		t.Errorf("expected '--max-tokens 500' in rewritten command, got %q", dec500.RewrittenCommand)
	}
	if dec500.TokenBudget != 500 {
		t.Errorf("expected TokenBudget=500, got %d", dec500.TokenBudget)
	}

	// Explicit 4000 tokens
	req4000 := &RouteRequest{
		CommandLine: "grep -r \"func\" pkg/",
		ProjectRoot: tmpDir,
		TokenBudget: 4000,
	}
	dec4000, err := gr.Route(context.Background(), req4000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(dec4000.RewrittenCommand, "--max-tokens 4000") {
		t.Errorf("expected '--max-tokens 4000' in rewritten command, got %q", dec4000.RewrittenCommand)
	}

	// Default budget when TokenBudget <= 0
	reqDef := &RouteRequest{
		CommandLine: "grep -r \"func\" pkg/",
		ProjectRoot: tmpDir,
		TokenBudget: 0,
	}
	decDef, err := gr.Route(context.Background(), reqDef)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(decDef.RewrittenCommand, "--max-tokens 2000") {
		t.Errorf("expected default '--max-tokens 2000' in rewritten command, got %q", decDef.RewrittenCommand)
	}
}

func TestGradientRouter_Route_TelemetryAndBypassReasons(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	gr := NewGradientRouter(nil, NewToolScope(tmpDir))

	// 1. In-repo query
	_, _ = gr.Route(context.Background(), &RouteRequest{
		CommandLine: "grep -rn \"find_me\" pkg/",
		ProjectRoot: tmpDir,
	})

	// 2. External path bypass
	_, _ = gr.Route(context.Background(), &RouteRequest{
		CommandLine: "grep \"outside\" /tmp/another.log",
		ProjectRoot: tmpDir,
	})

	// 3. Unsupported flags
	_, _ = gr.Route(context.Background(), &RouteRequest{
		CommandLine: "grep -v \"inverted\" pkg/",
		ProjectRoot: tmpDir,
	})

	stats := gr.Stats()
	if stats.RouteRequestsTotal != 3 {
		t.Errorf("expected RouteRequestsTotal=3, got %d", stats.RouteRequestsTotal)
	}
	if stats.RoutesChosen["zqk grep"] != 1 {
		t.Errorf("expected RoutesChosen['zqk grep']=1, got %d", stats.RoutesChosen["zqk grep"])
	}
	if stats.RoutesChosen["system"] != 2 {
		t.Errorf("expected RoutesChosen['system']=2, got %d", stats.RoutesChosen["system"])
	}
	if stats.BypassReasons[ReasonExternalPath] != 1 {
		t.Errorf("expected BypassReasons[external_path]=1, got %d", stats.BypassReasons[ReasonExternalPath])
	}
	if stats.BypassReasons[ReasonUnsupportedFlags] != 1 {
		t.Errorf("expected BypassReasons[unsupported_flags]=1, got %d", stats.BypassReasons[ReasonUnsupportedFlags])
	}
}

func TestGradientRouter_Route_EdgeCasesAndFallbacks(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	gr := NewGradientRouter(nil, NewToolScope(tmpDir))

	// Nil request
	_, err := gr.Route(context.Background(), nil)
	if err == nil {
		t.Fatalf("expected error on nil RouteRequest")
	}

	// Empty request
	decEmpty, err := gr.Route(context.Background(), &RouteRequest{})
	if err != nil {
		t.Fatalf("unexpected error on empty request: %v", err)
	}
	if !decEmpty.Bypassed || decEmpty.Reason != ReasonEmptyRequest {
		t.Errorf("expected Bypassed=true and Reason=%s, got %v", ReasonEmptyRequest, decEmpty)
	}

	// Non-search command passthrough
	decCat, err := gr.Route(context.Background(), &RouteRequest{
		CommandLine: "cat README.md",
		ProjectRoot: tmpDir,
	})
	if err != nil {
		t.Fatalf("unexpected error on cat command: %v", err)
	}
	if decCat.Intercepted {
		t.Errorf("expected Intercepted=false for cat command")
	}
	if !decCat.Fallback || decCat.TargetTool != "system" {
		t.Errorf("expected Fallback=true and TargetTool='system', got %v", decCat)
	}

	// Stream pipeline (grep in pipe)
	decPipe, err := gr.Route(context.Background(), &RouteRequest{
		CommandLine: "ps aux | grep zqk",
		ProjectRoot: tmpDir,
	})
	if err != nil {
		t.Fatalf("unexpected error on pipe command: %v", err)
	}
	if decPipe.Intercepted {
		t.Errorf("expected Intercepted=false for stream pipeline")
	}
	if decPipe.Reason != ReasonStreamPipeline {
		t.Errorf("expected Reason=%s, got %s", ReasonStreamPipeline, decPipe.Reason)
	}
}

func TestGradientRouter_Route_ConcurrentStress(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	gr := NewGradientRouter(nil, NewToolScope(tmpDir))

	const workers = 12
	const iterations = 40
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("gradient_router_route_worker", "stress_test").StartSimple(func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				var cmdLine string
				switch j % 3 {
				case 0:
					cmdLine = "grep -rn 'func' pkg/"
				case 1:
					cmdLine = "grep 'foo' /tmp/outside.txt"
				case 2:
					cmdLine = "grep -v 'bad' pkg/"
				}
				_, _ = gr.Route(context.Background(), &RouteRequest{
					CommandLine: cmdLine,
					ProjectRoot: tmpDir,
					TokenBudget: 1000 + (j * 10),
				})
			}
		})
	}

	wg.Wait()

	stats := gr.Stats()
	expectedTotal := uint64(workers * iterations)
	if stats.RouteRequestsTotal != expectedTotal {
		t.Errorf("expected RouteRequestsTotal=%d, got %d", expectedTotal, stats.RouteRequestsTotal)
	}
}
