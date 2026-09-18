package mcp

import (
	"context"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

// TestInProcessCLIRunner_WhenSet_ServerUsesIt verifies that when an in-process CLI runner is set,
// executeCLICommandWithContext uses it and returns the runner's result (no subprocess).
func TestInProcessCLIRunner_WhenSet_ServerUsesIt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	server := NewServer()
	server.SetCliInitializationContext(pkgctx.NewCliInitializationContext(func(string) string { return "/tmp" }, "."))
	server.SetSecurityContext(pkgctx.NewSystemSecurityContext())

	server.SetInProcessCLIRunner(func(_ context.Context, commandPath string, cmdArgs []string) (any, error) {
		result := make(map[string]any)
		result["in_process"] = true
		result[objects.FieldKeyCommand] = commandPath
		return result, nil
	})

	args := map[string]any{
		"_command_path":        "object list",
		objects.FieldKeyFormat: "json",
	}
	got, err := server.executeCLICommandWithContext(ctx, args)
	if err != nil {
		t.Fatalf("executeCLICommandWithContext: %v", err)
	}
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", got)
	}
	if m["in_process"] != true {
		t.Errorf("expected in_process=true, got %v", m["in_process"])
	}
	if m[objects.FieldKeyCommand] != "object list" {
		t.Errorf("expected command=object list, got %v", m[objects.FieldKeyCommand])
	}
}
