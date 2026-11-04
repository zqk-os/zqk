// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package router

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// TestCycle204_ConstantsAndNormalization verifies string normalization and sentinel errors.
func TestCycle204_ConstantsAndNormalization(t *testing.T) {
	t.Parallel()

	if got := normalizeKey("  Gemini-Flash-1.5  "); got != "gemini-flash-1.5" {
		t.Fatalf("unexpected normalized key: %q", got)
	}

	sentinels := []error{
		ErrNilRequest,
		ErrEmptyModel,
		ErrModelNotFound,
		ErrModelTimeout,
		ErrAllTiersFailed,
		ErrNilToolCall,
		ErrEmptyToolName,
		ErrToolNotFound,
		ErrOutOfScope,
		ErrPermissionDenied,
		ErrPathTraversalDetected,
		ErrReadOnlyViolation,
		ErrNilHandler,
	}

	for _, err := range sentinels {
		if err == nil || err.Error() == "" {
			t.Fatalf("expected non-empty sentinel error, got: %v", err)
		}
	}
}

// TestCycle204_NilHandlerBinding verifies bindHandler correctly ignores nil handlers.
func TestCycle204_NilHandlerBinding(t *testing.T) {
	t.Parallel()

	mr := NewModelRouter(nil)
	mr.RegisterModel("test-model", nil)
	mr.RegisterTier(TierFast, nil)

	tr := NewToolRouter(nil)
	tr.RegisterTool("test-tool", nil)

	if h := mr.resolveHandler("test-model", TierFast); h != nil {
		t.Fatalf("expected nil handler when registered with nil")
	}
}

// TestCycle204_TimeoutEnvelope_NilMapAndZeroDurations verifies edge branches of TimeoutEnvelope.
func TestCycle204_TimeoutEnvelope_NilMapAndZeroDurations(t *testing.T) {
	t.Parallel()

	env := &TimeoutEnvelope{}
	env.WithBaseTimeout(TierFast, 2*time.Second)
	env.WithBounds(500*time.Millisecond, 10*time.Second)
	env.WithTokenHeadroom(5 * time.Millisecond)
	env.WithByteHeadroom(2 * time.Microsecond)

	req := &ModelRequest{
		Prompt:    "hello world",
		MaxTokens: 100,
	}

	timeoutFast := env.ComputeTimeout(TierFast, req)
	if timeoutFast <= 0 {
		t.Fatalf("expected positive timeout, got %v", timeoutFast)
	}

	// Unknown tier or zero base timeout falls back to DefaultStandardTimeout
	timeoutUnknown := env.ComputeTimeout(ModelTier("non-existent"), req)
	if timeoutUnknown <= 0 {
		t.Fatalf("expected positive fallback timeout, got %v", timeoutUnknown)
	}

	// Verify WithTimeoutContext cancellation and deadline expiration
	ctx, cancel, dur := env.WithTimeoutContext(context.Background(), TierFast, req)
	defer cancel()

	if dur != timeoutFast {
		t.Fatalf("expected dur %v, got %v", timeoutFast, dur)
	}
	if ctx.Err() != nil {
		t.Fatalf("unexpected early context error: %v", ctx.Err())
	}
}

// TestCycle204_TimeoutEnvelope_ClampingBounds verifies min and max timeout boundaries.
func TestCycle204_TimeoutEnvelope_ClampingBounds(t *testing.T) {
	t.Parallel()

	env := NewTimeoutEnvelope()
	env.WithBounds(5*time.Second, 10*time.Second)
	env.WithBaseTimeout(TierFast, 100*time.Millisecond)

	lowTimeout := env.ComputeTimeout(TierFast, nil)
	if lowTimeout != 5*time.Second {
		t.Fatalf("expected clamped min timeout 5s, got %v", lowTimeout)
	}

	env.WithBaseTimeout(TierHeavy, 30*time.Second)
	highTimeout := env.ComputeTimeout(TierHeavy, nil)
	if highTimeout != 10*time.Second {
		t.Fatalf("expected clamped max timeout 10s, got %v", highTimeout)
	}
}

// TestCycle204_ModelRouter_ValidationErrors tests parameter validation for RouteModel.
func TestCycle204_ModelRouter_ValidationErrors(t *testing.T) {
	t.Parallel()

	mr := NewModelRouter(nil)

	resNil, errNil := mr.RouteModel(context.Background(), nil)
	if !errors.Is(errNil, ErrNilRequest) || resNil != nil {
		t.Fatalf("expected ErrNilRequest, got res=%v, err=%v", resNil, errNil)
	}

	resEmpty, errEmpty := mr.RouteModel(context.Background(), &ModelRequest{Model: "   "})
	if !errors.Is(errEmpty, ErrEmptyModel) || resEmpty != nil {
		t.Fatalf("expected ErrEmptyModel, got res=%v, err=%v", resEmpty, errEmpty)
	}
}

// TestCycle204_ModelRouter_FallbackGradientMatrix tests multi-stage fallback escalation.
func TestCycle204_ModelRouter_FallbackGradientMatrix(t *testing.T) {
	t.Parallel()

	mr := NewModelRouter(nil)

	mr.RegisterTier(TierFast, func(ctx context.Context, req *ModelRequest) (*ModelResponse, error) {
		return nil, context.DeadlineExceeded
	})
	mr.RegisterTier(TierStandard, func(ctx context.Context, req *ModelRequest) (*ModelResponse, error) {
		return nil, errors.New("upstream standard rate limited")
	})
	mr.RegisterTier(TierHeavy, func(ctx context.Context, req *ModelRequest) (*ModelResponse, error) {
		return &ModelResponse{
			Content:    "heavy-fallback-content",
			TokensUsed: 120,
		}, nil
	})

	req := &ModelRequest{
		Model:         "claude-haiku",
		PreferredTier: TierFast,
		FallbackChain: []ModelTier{
			TierFast,
			ModelTier("invalid-tier"),
			TierStandard,
			TierHeavy,
		},
	}

	resp, err := mr.RouteModel(context.Background(), req)
	if err != nil {
		t.Fatalf("expected successful fallback to TierHeavy, got error: %v", err)
	}
	if resp.Tier != TierHeavy {
		t.Fatalf("expected TierHeavy, got %q", resp.Tier)
	}
	if resp.AttemptCount != 3 {
		t.Fatalf("expected 3 attempts before success, got %d", resp.AttemptCount)
	}
	if resp.Content != "heavy-fallback-content" {
		t.Fatalf("unexpected response content: %q", resp.Content)
	}
}

// TestCycle204_ModelRouter_AllTiersFailureModes tests comprehensive fallback exhaustions.
func TestCycle204_ModelRouter_AllTiersFailureModes(t *testing.T) {
	t.Parallel()

	t.Run("EmptyResponseOnTier", func(t *testing.T) {
		mr := NewModelRouter(nil)
		mr.RegisterTier(TierFast, func(ctx context.Context, req *ModelRequest) (*ModelResponse, error) {
			return nil, nil // empty response without error
		})

		_, err := mr.RouteModel(context.Background(), &ModelRequest{
			Model:         "fast-model",
			PreferredTier: TierFast,
		})
		if !errors.Is(err, ErrAllTiersFailed) {
			t.Fatalf("expected ErrAllTiersFailed, got %v", err)
		}
	})

	t.Run("MissingHandlerOnTier", func(t *testing.T) {
		mr := NewModelRouter(nil)
		_, err := mr.RouteModel(context.Background(), &ModelRequest{
			Model:         "custom-unregistered",
			PreferredTier: TierFast,
		})
		if !errors.Is(err, ErrAllTiersFailed) {
			t.Fatalf("expected ErrAllTiersFailed, got %v", err)
		}
	})
}

// TestCycle204_ModelRouter_DefaultCatchAllHandler verifies the catch-all default handler.
func TestCycle204_ModelRouter_DefaultCatchAllHandler(t *testing.T) {
	t.Parallel()

	mr := NewModelRouter(nil)
	mr.SetDefaultHandler(func(ctx context.Context, req *ModelRequest) (*ModelResponse, error) {
		return &ModelResponse{
			Content:    "universal-catchall:" + req.Model,
			TokensUsed: 15,
		}, nil
	})

	resp, err := mr.RouteModel(context.Background(), &ModelRequest{
		Model: "unregistered-provider/unregistered-model",
	})
	if err != nil {
		t.Fatalf("unexpected error with default handler: %v", err)
	}
	if resp.Content != "universal-catchall:unregistered-provider/unregistered-model" {
		t.Fatalf("unexpected response: %q", resp.Content)
	}
}

// TestCycle204_ToolScope_NilAndEmptyPath tests boundary conditions on ToolScope.
func TestCycle204_ToolScope_NilAndEmptyPath(t *testing.T) {
	t.Parallel()

	var nilScope *ToolScope
	if err := nilScope.ValidatePath("some/path", true); err != nil {
		t.Fatalf("expected nil error on nil scope, got: %v", err)
	}

	scope := NewToolScope("/tmp/workspace")
	if err := scope.ValidatePath("   ", true); err != nil {
		t.Fatalf("expected nil error on empty path, got: %v", err)
	}

	// Empty candidate roots and empty workspace root in strict mode
	emptyRootScope := &ToolScope{Mode: ScopeModeStrict}
	if err := emptyRootScope.ValidatePath("/some/path", false); !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("expected ErrOutOfScope on empty roots, got: %v", err)
	}
}

// TestCycle204_ToolScope_TraversalAndPatterns tests path traversal and blocked patterns.
func TestCycle204_ToolScope_TraversalAndPatterns(t *testing.T) {
	t.Parallel()

	scope := NewToolScope("/workspace")

	traversalPaths := []string{
		"../secret",
		"sub/../../secret",
		"/workspace/../etc/passwd",
	}
	for _, p := range traversalPaths {
		err := scope.ValidatePath(p, false)
		if !errors.Is(err, ErrPathTraversalDetected) {
			t.Fatalf("expected ErrPathTraversalDetected for %q, got: %v", p, err)
		}
	}

	blockedPaths := []string{
		"/workspace/.git/config",
		"/workspace/id_rsa",
		"/workspace/keys/id_ed25519",
		"/workspace/.env",
		"/workspace/.git/hooks/pre-commit",
	}
	for _, p := range blockedPaths {
		err := scope.ValidatePath(p, false)
		if !errors.Is(err, ErrPermissionDenied) {
			t.Fatalf("expected ErrPermissionDenied for %q, got: %v", p, err)
		}
	}
}

// TestCycle204_ToolScope_ReadOnlyViolation tests write operations in read-only mode.
func TestCycle204_ToolScope_ReadOnlyViolation(t *testing.T) {
	t.Parallel()

	scope := NewToolScope("/workspace")
	scope.Mode = ScopeModeReadOnly

	if err := scope.ValidatePath("/workspace/doc.md", false); err != nil {
		t.Fatalf("read operation should succeed in read_only mode, got: %v", err)
	}

	err := scope.ValidatePath("/workspace/doc.md", true)
	if !errors.Is(err, ErrReadOnlyViolation) {
		t.Fatalf("expected ErrReadOnlyViolation for write, got: %v", err)
	}
}

// TestCycle204_ToolRouter_TargetPathsExtraction tests path extraction from arguments.
func TestCycle204_ToolRouter_TargetPathsExtraction(t *testing.T) {
	t.Parallel()

	if paths := ExtractTargetPaths(nil); len(paths) != 0 {
		t.Fatalf("expected empty paths for nil args, got: %v", paths)
	}

	args := map[string]any{
		ArgPath:         "/path/one.txt",
		ArgFilePath:     "/path/two.txt",
		ArgTargetFile:   "/path/three.txt",
		ArgAbsolutePath: "/path/four.txt",
		ArgCwd:          "/path/cwd",
		"ignore_num":    42,
		"empty_path":    "   ",
	}

	paths := ExtractTargetPaths(args)
	if len(paths) != 5 {
		t.Fatalf("expected 5 extracted paths, got %d: %v", len(paths), paths)
	}
}

// TestCycle204_ToolRouter_ScopeMutationAndInterceptors tests scope mutation and interceptors.
func TestCycle204_ToolRouter_ScopeMutationAndInterceptors(t *testing.T) {
	t.Parallel()

	tr := NewToolRouter(NewToolScope("/tmp/root1"))
	tr.SetScope(NewToolScope("/tmp/root2"))
	tr.AddInterceptor(nil) // nil interceptor guard check

	// ScopeVerificationInterceptor direct unit invocation
	resNilCall, errNilCall := ScopeVerificationInterceptor(context.Background(), nil, nil)
	if !errors.Is(errNilCall, ErrNilToolCall) || resNilCall != nil {
		t.Fatalf("expected ErrNilToolCall on nil call, got res=%v, err=%v", resNilCall, errNilCall)
	}

	callNoScope := &ToolCall{
		ToolName:    ToolReadFile,
		TargetPaths: []string{"/safe/path"},
	}
	resPass, errPass := ScopeVerificationInterceptor(context.Background(), callNoScope, func(ctx context.Context, c *ToolCall) (*ToolResult, error) {
		return &ToolResult{ToolName: c.ToolName, Output: "passed"}, nil
	})
	if errPass != nil || resPass == nil || resPass.Output != "passed" {
		t.Fatalf("expected next handler output, got res=%v, err=%v", resPass, errPass)
	}
}

// TestCycle204_ToolRouter_PassthroughDisabled tests rejection when passthrough is disabled.
func TestCycle204_ToolRouter_PassthroughDisabled(t *testing.T) {
	t.Parallel()

	scope := NewToolScope("/tmp/workspace")
	scope.AllowPassthrough = false

	tr := NewToolRouter(scope)
	tr.SetDefaultHandler(func(ctx context.Context, call *ToolCall) (*ToolResult, error) {
		return &ToolResult{Output: "default"}, nil
	})

	_, err := tr.RouteTool(context.Background(), &ToolCall{
		ToolName: "unregistered_external_tool",
	})
	if !errors.Is(err, ErrToolNotFound) {
		t.Fatalf("expected ErrToolNotFound when passthrough is disabled, got: %v", err)
	}
}

// TestCycle204_GradientRouter_BuilderBranches tests all builder configuration paths.
func TestCycle204_GradientRouter_BuilderBranches(t *testing.T) {
	t.Parallel()

	// 1. Initializing WithAllowedRoots and WithScopeMode before WithWorkspaceRoot
	b1 := NewGradientRouterBuilder().
		WithAllowedRoots("/extra/root1", "/extra/root2").
		WithScopeMode(ScopeModePermissive).
		WithWorkspaceRoot("/primary/root")

	gr1 := b1.Build()
	if gr1 == nil {
		t.Fatalf("expected non-nil GradientRouter")
	}

	// 2. Initializing builder with WithWorkspaceRoot then adding allowed roots
	b2 := NewGradientRouterBuilder().
		WithWorkspaceRoot("/root1").
		WithWorkspaceRoot("/root2").
		WithAllowedRoots("/root3")

	gr2 := b2.Build()
	if gr2 == nil {
		t.Fatalf("expected non-nil GradientRouter")
	}

	// 3. Initializing WithScopeMode when scope is nil
	b3 := NewGradientRouterBuilder().
		WithScopeMode(ScopeModeReadOnly)
	gr3 := b3.Build()
	if gr3 == nil {
		t.Fatalf("expected non-nil GradientRouter")
	}

	// 4. Initializing with nil envelope directly
	grNilEnv := NewGradientRouter(nil, nil)
	if grNilEnv == nil {
		t.Fatalf("expected non-nil GradientRouter from NewGradientRouter(nil, nil)")
	}
}

// TestCycle204_GradientRouter_FallbackAndInterceptedTelemetry verifies telemetry counters.
func TestCycle204_GradientRouter_FallbackAndInterceptedTelemetry(t *testing.T) {
	t.Parallel()

	gr := NewGradientRouter(nil, NewToolScope("/safe/dir"))

	// Register tiered model handlers where Fast fails and Standard succeeds
	gr.RegisterTier(TierFast, func(ctx context.Context, req *ModelRequest) (*ModelResponse, error) {
		return nil, errors.New("tier fast unavailable")
	})
	gr.RegisterTier(TierStandard, func(ctx context.Context, req *ModelRequest) (*ModelResponse, error) {
		return &ModelResponse{
			Content: "standard-success",
		}, nil
	})

	mResp, mErr := gr.RouteModel(context.Background(), &ModelRequest{
		Model:         "claude-fast",
		PreferredTier: TierFast,
		FallbackChain: []ModelTier{TierStandard},
	})
	if mErr != nil || mResp == nil {
		t.Fatalf("expected model route fallback success, got resp=%v, err=%v", mResp, mErr)
	}

	// Register ToolWriteFile so tool routing resolves handler and executes ScopeVerificationInterceptor
	gr.RegisterTool(ToolWriteFile, func(ctx context.Context, call *ToolCall) (*ToolResult, error) {
		return &ToolResult{
			ToolName: call.ToolName,
			Output:   "written",
		}, nil
	})

	// Route an out-of-scope tool call that produces an intercepted error
	tRes, tErr := gr.RouteTool(context.Background(), &ToolCall{
		ToolName: ToolWriteFile,
		Arguments: map[string]any{
			ArgPath: "/unsafe/system/file",
		},
	})
	if tErr == nil || tRes == nil || !tRes.Intercepted {
		t.Fatalf("expected tool interception, got res=%v, err=%v", tRes, tErr)
	}

	// Route a nil tool call resulting in nil result and non-nil error (increments toolIntercepted)
	nilRes, nilErr := gr.RouteTool(context.Background(), nil)
	if nilRes != nil || !errors.Is(nilErr, ErrNilToolCall) {
		t.Fatalf("expected ErrNilToolCall, got res=%v, err=%v", nilRes, nilErr)
	}

	stats := gr.Stats()
	if stats.FallbackEventsTotal != 1 {
		t.Fatalf("expected 1 fallback event, got %d", stats.FallbackEventsTotal)
	}
	if stats.ToolCallsIntercepted != 2 {
		t.Fatalf("expected 2 intercepted tool calls, got %d", stats.ToolCallsIntercepted)
	}
	if stats.ModelRequestsByTier[TierStandard] != 1 {
		t.Fatalf("expected 1 request on TierStandard, got %d", stats.ModelRequestsByTier[TierStandard])
	}
}

// TestCycle204_ModelTier_ClassifyModelTierHeuristics checks edge cases in model classification.
func TestCycle204_ModelTier_ClassifyModelTierHeuristics(t *testing.T) {
	t.Parallel()

	cases := []struct {
		model    string
		expected ModelTier
	}{
		{"", TierStandard},
		{"   ", TierStandard},
		{"ollama/llama3", TierLocal},
		{"local/custom-coder", TierLocal},
		{"deepseek-local-7b", TierLocal},
		{"qwen-local-32b", TierLocal},
		{"mistral-local", TierLocal},
		{"phi-3-mini", TierLocal}, // 'phi' local family takes precedence
		{"qwen-2.5-small", TierFast},
		{"gemini-2.5-flash", TierFast},
		{"claude-3-haiku", TierFast},
		{"gpt-4o-mini", TierFast},
		{"o1-preview", TierHeavy},
		{"o3-mini", TierHeavy},
		{"claude-opus-20240229", TierHeavy},
		{"deepseek-r1", TierHeavy},
		{"qwen-qwq-32b", TierHeavy},
		{"gemini-1.5-pro", TierStandard},
		{"claude-3-5-sonnet", TierStandard},
	}

	for _, c := range cases {
		tier := ClassifyModelTier(c.model)
		if tier != c.expected {
			t.Errorf("ClassifyModelTier(%q) = %q, expected %q", c.model, tier, c.expected)
		}
	}
}

// TestCycle204_ConcurrentModelAndToolRouting stress tests multi-goroutine routing.
func TestCycle204_ConcurrentModelAndToolRouting(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	gr := NewGradientRouterBuilder().
		WithWorkspaceRoot(tmpDir).
		WithDefaultModelHandler(func(ctx context.Context, req *ModelRequest) (*ModelResponse, error) {
			return &ModelResponse{
				Content:    "concurrent-resp-" + req.Model,
				TokensUsed: 20,
			}, nil
		}).
		WithDefaultToolHandler(func(ctx context.Context, call *ToolCall) (*ToolResult, error) {
			return &ToolResult{
				ToolName: call.ToolName,
				Output:   "concurrent-tool-ok",
			}, nil
		}).
		Build()

	const concurrency = 20
	const operations = 25
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		idx := i
		goroutinelabels.NewGoroutine("cycle204_router_worker", "stress_test").StartSimple(func() {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				// Concurrent model routing
				modelReq := &ModelRequest{
					Model: fmt.Sprintf("model-worker-%d-%d", idx, j),
				}
				mResp, mErr := gr.RouteModel(context.Background(), modelReq)
				if mErr != nil || mResp == nil {
					t.Errorf("concurrent RouteModel failed: %v", mErr)
					return
				}

				// Concurrent tool routing
				toolCall := &ToolCall{
					ToolName: ToolReadFile,
					Arguments: map[string]any{
						ArgPath: filepath.Join(tmpDir, fmt.Sprintf("worker-%d-file-%d.txt", idx, j)),
					},
				}
				tRes, tErr := gr.RouteTool(context.Background(), toolCall)
				if tErr != nil || tRes == nil {
					t.Errorf("concurrent RouteTool failed: %v", tErr)
					return
				}
			}
		})
	}

	wg.Wait()

	stats := gr.Stats()
	expectedTotal := uint64(concurrency * operations)
	if stats.ModelRequestsTotal != expectedTotal {
		t.Fatalf("expected %d model requests, got %d", expectedTotal, stats.ModelRequestsTotal)
	}
	if stats.ToolCallsTotal != expectedTotal {
		t.Fatalf("expected %d tool calls, got %d", expectedTotal, stats.ToolCallsTotal)
	}
}
