package router

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestToolRouter_ScopeValidation_InScope(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	scope := NewToolScope(tmpDir)
	router := NewToolRouter(scope)

	executed := false
	router.RegisterTool(ToolReadFile, func(ctx context.Context, call *ToolCall) (*ToolResult, error) {
		executed = true
		return &ToolResult{
			ToolName: call.ToolName,
			Output:   "file contents",
		}, nil
	})

	validPath := filepath.Join(tmpDir, "src", "main.go")
	res, err := router.RouteTool(context.Background(), &ToolCall{
		ToolName: ToolReadFile,
		Arguments: map[string]any{
			ArgPath: validPath,
		},
	})
	if err != nil {
		t.Fatalf("expected successful execution, got error: %v", err)
	}
	if !executed {
		t.Fatalf("expected handler to execute")
	}
	if res.Output != "file contents" {
		t.Errorf("expected Output=%q, got %v", "file contents", res.Output)
	}
}

func TestToolRouter_ScopeValidation_OutOfScope(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	scope := NewToolScope(tmpDir)
	router := NewToolRouter(scope)

	router.RegisterTool(ToolReadFile, func(ctx context.Context, call *ToolCall) (*ToolResult, error) {
		return &ToolResult{Output: "should not be reached"}, nil
	})

	outOfScopeFile := filepath.Join(string(filepath.Separator), "etc", "passwd")
	res, err := router.RouteTool(context.Background(), &ToolCall{
		ToolName: ToolReadFile,
		Arguments: map[string]any{
			ArgPath: outOfScopeFile,
		},
	})
	if !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("expected ErrOutOfScope, got %v", err)
	}
	if res == nil || !res.Intercepted {
		t.Fatalf("expected ToolResult to record Intercepted=true")
	}
}

func TestToolRouter_ScopeValidation_PathTraversal(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	scope := NewToolScope(tmpDir)
	router := NewToolRouter(scope)

	router.RegisterTool(ToolReadFile, func(ctx context.Context, call *ToolCall) (*ToolResult, error) {
		return &ToolResult{Output: "should not run"}, nil
	})

	traversalPath := tmpDir + string(filepath.Separator) + ".." + string(filepath.Separator) + "other" + string(filepath.Separator) + "secret.txt"
	_, err := router.RouteTool(context.Background(), &ToolCall{
		ToolName:    ToolReadFile,
		TargetPaths: []string{traversalPath},
	})
	if !errors.Is(err, ErrPathTraversalDetected) {
		t.Fatalf("expected ErrPathTraversalDetected, got %v", err)
	}
}

func TestToolRouter_ScopeValidation_BlockedPattern(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	scope := NewToolScope(tmpDir)
	router := NewToolRouter(scope)

	router.RegisterTool(ToolReadFile, func(ctx context.Context, call *ToolCall) (*ToolResult, error) {
		return &ToolResult{Output: "blocked"}, nil
	})

	blockedFile := filepath.Join(tmpDir, ".env")
	_, err := router.RouteTool(context.Background(), &ToolCall{
		ToolName: ToolReadFile,
		Arguments: map[string]any{
			ArgPath: blockedFile,
		},
	})
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("expected ErrPermissionDenied for blocked file, got %v", err)
	}
}

func TestToolRouter_ScopeValidation_ReadOnlyViolation(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	scope := NewToolScope(tmpDir)
	scope.Mode = ScopeModeReadOnly
	router := NewToolRouter(scope)

	router.RegisterTool(ToolWriteFile, func(ctx context.Context, call *ToolCall) (*ToolResult, error) {
		return &ToolResult{Output: "wrote"}, nil
	})
	router.RegisterTool(ToolReadFile, func(ctx context.Context, call *ToolCall) (*ToolResult, error) {
		return &ToolResult{Output: "read ok"}, nil
	})

	target := filepath.Join(tmpDir, "data.txt")

	// 1. Read operation allowed in ReadOnly mode
	resRead, err := router.RouteTool(context.Background(), &ToolCall{
		ToolName: ToolReadFile,
		Arguments: map[string]any{
			ArgPath: target,
		},
	})
	if err != nil || resRead.Output != "read ok" {
		t.Fatalf("expected read to succeed in ReadOnly mode, got err: %v", err)
	}

	// 2. Write operation blocked in ReadOnly mode
	_, err = router.RouteTool(context.Background(), &ToolCall{
		ToolName: ToolWriteFile,
		Arguments: map[string]any{
			ArgPath: target,
		},
	})
	if !errors.Is(err, ErrReadOnlyViolation) {
		t.Fatalf("expected ErrReadOnlyViolation, got %v", err)
	}
}

func TestToolRouter_ScopeValidation_Permissive(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	scope := NewToolScope(tmpDir)
	scope.Mode = ScopeModePermissive
	router := NewToolRouter(scope)

	router.RegisterTool(ToolReadFile, func(ctx context.Context, call *ToolCall) (*ToolResult, error) {
		return &ToolResult{Output: "permissive ok"}, nil
	})

	outOfScope := filepath.Join(string(filepath.Separator), "tmp", "arbitrary.txt")
	res, err := router.RouteTool(context.Background(), &ToolCall{
		ToolName: ToolReadFile,
		Arguments: map[string]any{
			ArgPath: outOfScope,
		},
	})
	if err != nil {
		t.Fatalf("expected permissive mode to allow path, got: %v", err)
	}
	if res.Output != "permissive ok" {
		t.Errorf("unexpected output: %v", res.Output)
	}
}

func TestToolRouter_PathPassthrough(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	scope := NewToolScope(tmpDir)
	router := NewToolRouter(scope)

	router.SetDefaultHandler(func(ctx context.Context, call *ToolCall) (*ToolResult, error) {
		return &ToolResult{
			ToolName: call.ToolName,
			Output:   "passthrough: " + call.ToolName,
		}, nil
	})

	// Invoke unregistered external tool
	res, err := router.RouteTool(context.Background(), &ToolCall{
		ToolName: "custom_external_api",
		Arguments: map[string]any{
			"url": "https://api.example.com",
		},
	})
	if err != nil {
		t.Fatalf("RouteTool passthrough failed: %v", err)
	}
	if !res.Passthrough {
		t.Fatalf("expected Passthrough=true on default tool handler invocation")
	}
	if res.Output != "passthrough: custom_external_api" {
		t.Errorf("unexpected output: %v", res.Output)
	}
}

func TestToolRouter_CustomInterceptorPipeline(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	scope := NewToolScope(tmpDir)
	router := NewToolRouter(scope)

	interceptorRan := false
	router.AddInterceptor(func(ctx context.Context, call *ToolCall, next ToolHandler) (*ToolResult, error) {
		interceptorRan = true
		call.Arguments["injected_flag"] = true
		return next(ctx, call)
	})

	router.RegisterTool("check_injection", func(ctx context.Context, call *ToolCall) (*ToolResult, error) {
		val := call.Arguments["injected_flag"]
		return &ToolResult{Output: val}, nil
	})

	res, err := router.RouteTool(context.Background(), &ToolCall{
		ToolName:  "check_injection",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if !interceptorRan {
		t.Fatalf("expected custom interceptor to run")
	}
	if res.Output != true {
		t.Fatalf("expected injected argument to be observed by handler")
	}
}

func TestToolRouter_BoundaryErrors(t *testing.T) {
	t.Parallel()

	router := NewToolRouter(nil)

	// 1. Nil call
	if _, err := router.RouteTool(context.Background(), nil); !errors.Is(err, ErrNilToolCall) {
		t.Errorf("expected ErrNilToolCall, got %v", err)
	}

	// 2. Empty tool name
	if _, err := router.RouteTool(context.Background(), &ToolCall{ToolName: ""}); !errors.Is(err, ErrEmptyToolName) {
		t.Errorf("expected ErrEmptyToolName, got %v", err)
	}

	// 3. Tool not found and no passthrough handler
	_, err := router.RouteTool(context.Background(), &ToolCall{ToolName: "non_existent_tool"})
	if !errors.Is(err, ErrToolNotFound) {
		t.Errorf("expected ErrToolNotFound, got %v", err)
	}
}
