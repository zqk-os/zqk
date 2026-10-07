package cli

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp/context"
	"github.com/zqk-os/zqk/pkg/paths"
)

func TestContext_DerivationsAndGuards(t *testing.T) {
	// Nil receiver guards
	var nilCtx *Context
	if nilCtx.Derive(nil) != nil {
		t.Errorf("expected Derive on nil context to return nil")
	}
	if nilCtx.WithFormat("json") != nil {
		t.Errorf("expected WithFormat on nil context to return nil")
	}
	if nilCtx.WithVerbose(true) != nil {
		t.Errorf("expected WithVerbose on nil context to return nil")
	}
	if nilCtx.WithQuiet(true) != nil {
		t.Errorf("expected WithQuiet on nil context to return nil")
	}
	if nilCtx.WithProfile("ai-agent") != nil {
		t.Errorf("expected WithProfile on nil context to return nil")
	}
	if nilCtx.WithPathResolver(nil) != nil {
		t.Errorf("expected WithPathResolver on nil context to return nil")
	}
	sc := nilCtx.GetStorageContext()
	if sc == nil {
		t.Errorf("expected non-nil default storage context for nil Context")
	}
	pr := nilCtx.PathResolver()
	if pr == nil {
		t.Errorf("expected non-nil default PathResolver for nil Context")
	}

	// Active Context derivation
	base := ContextForProjectRoot("/mock/root")
	if base.ProjectRoot != "/mock/root" {
		t.Errorf("expected /mock/root, got %s", base.ProjectRoot)
	}

	jsonCtx := base.WithFormat("json")
	if jsonCtx.Format != FormatJSON {
		t.Errorf("expected FormatJSON, got %s", jsonCtx.Format)
	}

	verbCtx := base.WithVerbose(true)
	if !verbCtx.Verbose {
		t.Errorf("expected Verbose=true")
	}

	quietCtx := base.WithQuiet(true)
	if !quietCtx.Quiet {
		t.Errorf("expected Quiet=true")
	}

	profCtx := base.WithProfile("ai-agent")
	if profCtx.Profile != "ai-agent" {
		t.Errorf("expected Profile=ai-agent, got %s", profCtx.Profile)
	}

	// PathResolver derivation
	mockResolver := paths.NewPathResolver("/mock/root")
	withResolver := base.WithPathResolver(mockResolver)
	if withResolver == nil || withResolver.PathResolver() == nil {
		t.Errorf("expected non-nil path resolver")
	}

	// ContextForProjectAndProfile
	candp := ContextForProjectAndProfile("/mock/root", "debug")
	if candp == nil || candp.Profile != "debug" {
		t.Errorf("expected debug profile")
	}

	// ContextFromInner
	if ContextFromInner(nil) != nil {
		t.Errorf("expected ContextFromInner(nil) == nil")
	}
	inner := &context.Context{ProjectRoot: "/inner"}
	wrapped := ContextFromInner(inner)
	if wrapped == nil || wrapped.ProjectRoot != "/inner" {
		t.Errorf("expected wrapped context")
	}

	// NewContextManager
	cm := NewContextManager()
	if cm == nil {
		t.Errorf("expected NewContextManager to return non-nil")
	}
}

func TestResolveCommandProjectRoot(t *testing.T) {
	rootNil, err := ResolveCommandProjectRoot(nil)
	if err != nil || rootNil == "" {
		t.Errorf("expected fallback root for nil command, got %s (err: %v)", rootNil, err)
	}

	cmd := &cobra.Command{Use: "test"}
	ctx := ContextForProjectRoot("/test/proj/root")
	SetContext(cmd, ctx)
	root, err := ResolveCommandProjectRoot(cmd)
	if err != nil {
		t.Fatalf("unexpected error resolving project root: %v", err)
	}
	if root != "/test/proj/root" {
		t.Errorf("expected /test/proj/root, got %s", root)
	}
}
