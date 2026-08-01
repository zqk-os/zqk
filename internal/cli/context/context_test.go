package context

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestContextManager_LoadContext(t *testing.T) {
	manager := NewContextManager()

	// Test with no config files (should use system defaults)
	initCtx := pkgctx.NewCliInitializationContext(ResolveProjectRoot, ".")
	ctx, err := manager.LoadContext(initCtx)
	if err != nil {
		t.Fatalf("Failed to load context: %v", err)
	}

	// Verify defaults
	// Format may be overridden by config files or environment, so we check for reasonable defaults
	validFormats := []string{"table", "json", "yaml", "jsonl"}
	formatValid := false
	for _, f := range validFormats {
		if ctx.Format == f {
			formatValid = true
			break
		}
	}
	if !formatValid {
		t.Errorf("Expected default format to be one of %v, got %s", validFormats, ctx.Format)
	}
	// Verbose should default to false
	if ctx.Verbose {
		t.Error("Expected default verbose to be false")
	}
}

func TestContextManager_Profile(t *testing.T) {
	manager := NewContextManager()

	// Test ai-agent profile
	manager.SetCommandFlag("profile", "ai-agent")
	initCtx := pkgctx.NewCliInitializationContext(ResolveProjectRoot, ".")
	ctx, err := manager.LoadContext(initCtx)
	if err != nil {
		t.Fatalf("Failed to load context: %v", err)
	}

	// Profile should be applied from pkg/cli/profiles (jsonl for ai-agent)
	if ctx.Format != "jsonl" {
		t.Errorf("Expected ai-agent profile to set format to jsonl, got %s", ctx.Format)
	}
	// Profile name may be empty after application, but format should reflect profile settings
	// Format should be different from default "table"
	if ctx.Format == "table" {
		t.Errorf("Expected profile to change format from default 'table', but format is still 'table'")
	}
}

func TestContextManager_Precedence(t *testing.T) {
	manager := NewContextManager()

	// Set user config format
	manager.userConfig = map[string]any{
		objects.FieldKeyFormat: "yaml",
	}

	// Set command flag format (should override)
	manager.SetCommandFlag("format", "json")

	initCtx := pkgctx.NewCliInitializationContext(ResolveProjectRoot, ".")
	ctx, err := manager.LoadContext(initCtx)
	if err != nil {
		t.Fatalf("Failed to load context: %v", err)
	}

	// Command flag should win
	if ctx.Format != "json" {
		t.Errorf("Expected command flag to override user config, got %s", ctx.Format)
	}
}

func TestGetContextFromCommand(t *testing.T) {
	// Create a test command
	cmd := &cobra.Command{
		Use: "test",
	}
	cmd.Flags().String("format", "", "Format")
	cmd.Flags().Bool("verbose", false, "Verbose")
	cmd.Flags().String("context", "", "Context profile")

	// Set flags
	//nolint:errcheck // Test helper - flag set errors are acceptable
	_ = cmd.Flags().Set("format", "json")
	//nolint:errcheck // Test helper - flag set errors are acceptable
	_ = cmd.Flags().Set("context", "ai-agent")

	// Get context
	initCtx := pkgctx.NewCliInitializationContext(ResolveProjectRoot, ".")
	ctx, err := GetContextFromCommand(cmd, initCtx)
	if err != nil {
		t.Fatalf("Failed to get context: %v", err)
	}

	// Format should come from ai-agent profile (jsonl per pkg/cli/profiles/ai_agent.yaml)
	if ctx.Format != "jsonl" {
		t.Errorf("Expected format to be jsonl from ai-agent profile, got %s", ctx.Format)
	}
}

func TestResolveProjectRoot(t *testing.T) {
	tempDir := t.TempDir()

	// Create .zqk directory
	zqkDir := filepath.Join(tempDir, paths.ProjectDataDir)
	if err := os.MkdirAll(zqkDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create .zqk dir: %v", err)
	}

	// Create subdirectory
	subDir := filepath.Join(tempDir, "sub", "dir")
	if err := os.MkdirAll(subDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create sub dir: %v", err)
	}

	// Clear env so we test discovery, not env override (t.Setenv restores after the test).
	t.Setenv(zqkenv.ProjectRoot(), "")
	t.Setenv(zqkenv.TestRoot(), "")

	// Test discovery from subdirectory (ResolveProjectRoot falls back to findProjectRoot when env unset)
	root := ResolveProjectRoot(subDir)
	if root != tempDir {
		t.Errorf("Expected project root %s, got %s", tempDir, root)
	}

	// Topmost .zqk must win: if both repo/.zqk and repo/cmd/zqk/system/.zqk exist, resolve to repo
	nestedZqk := filepath.Join(tempDir, "cmd", "zqk", "system", paths.ProjectDataDir)
	if err := os.MkdirAll(nestedZqk, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create nested .zqk: %v", err)
	}
	nestedStart := filepath.Join(tempDir, "cmd", "zqk", "system")
	rootFromNested := ResolveProjectRoot(nestedStart)
	if rootFromNested != tempDir {
		t.Errorf("Expected project root %s when starting from nested dir (avoid cmd/.../.zqk), got %s", tempDir, rootFromNested)
	}
}

func TestGetErrorLogOutput(t *testing.T) {
	t.Run("nil context returns combined", func(t *testing.T) {
		var c *Context
		if got := c.GetErrorLogOutput(); got != "combined" {
			t.Errorf("GetErrorLogOutput(nil) = %q, want combined", got)
		}
	})
	t.Run("empty ErrorLogOutput returns combined", func(t *testing.T) {
		c := &Context{}
		if got := c.GetErrorLogOutput(); got != "combined" {
			t.Errorf("GetErrorLogOutput() = %q, want combined", got)
		}
	})
	t.Run("separate returns separate", func(t *testing.T) {
		c := &Context{ErrorLogOutput: "separate"}
		if got := c.GetErrorLogOutput(); got != "separate" {
			t.Errorf("GetErrorLogOutput() = %q, want separate", got)
		}
	})
}

func TestErrorLogOutputFromConfig(t *testing.T) {
	systemDefaults := map[string]any{
		objects.FieldKeyFormat: "table", "verbose": false, "quiet": false,
		"error_log_output": "combined",
	}
	projectConfig := map[string]any{
		"logging": map[string]any{
			"error_log_output": "separate",
		},
	}
	ctx, err := BuildContextFromLayers(systemDefaults, nil, projectConfig, nil, "/tmp/proj")
	if err != nil {
		t.Fatalf("BuildContextFromLayers: %v", err)
	}
	if got := ctx.GetErrorLogOutput(); got != "separate" {
		t.Errorf("project config logging.error_log_output=separate: GetErrorLogOutput() = %q, want separate", got)
	}
}

func TestContext_PathResolver(t *testing.T) {
	t.Parallel()
	c := &Context{ProjectRoot: "/tmp/zqk-proj"}
	r := c.PathResolver()
	if r.ProjectRoot() != "/tmp/zqk-proj" {
		t.Errorf("PathResolver.ProjectRoot() = %q", r.ProjectRoot())
	}
	paths.ReplacePathCache("/tmp/zqk-proj", paths.DefaultPathAliases())
	got, err := r.ResolveStrict(paths.PathSchemePrefix + "cache")
	if err != nil {
		t.Fatalf("ResolveStrict: %v", err)
	}
	want := filepath.Join("/tmp/zqk-proj", paths.ProjectDataDir, paths.CacheDir)
	if got != want {
		t.Errorf("ResolveStrict(cache) = %q want %q", got, want)
	}
}

type stubPathResolver struct {
	root string
}

func (s *stubPathResolver) ProjectRoot() string { return s.root }

func (s *stubPathResolver) ResolveStrict(string) (string, error) {
	return "/stub/process", nil
}

func (s *stubPathResolver) ResolveFromCacheOrConstant(_, fallbackRel string) string {
	return "/stub/" + fallbackRel
}

func TestContext_WithPathResolver(t *testing.T) {
	t.Parallel()
	base := &Context{ProjectRoot: "/tmp/p"}
	stub := &stubPathResolver{root: "/tmp/p"}
	derived := base.WithPathResolver(stub)
	if derived.PathResolver() != stub {
		t.Fatal("expected injected PathResolver")
	}
	cleared := derived.WithPathResolver(nil)
	if cleared.PathResolver() == stub {
		t.Fatal("expected override cleared")
	}
	if cleared.PathResolver().ProjectRoot() != "/tmp/p" {
		t.Fatalf("ProjectRoot after clear: %q", cleared.PathResolver().ProjectRoot())
	}
}
