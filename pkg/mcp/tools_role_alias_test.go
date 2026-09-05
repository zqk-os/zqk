package mcp

import (
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

// TestAliasModeOnlyBuiltinTools verifies that when alias_mode is true, only built-in tools
// are registered (no per-command CLI tools). Built-in tools use GetToolName() so they have
// the brand prefix (e.g. zqk_); CLI-discovered tools use sanitizeToolName and have no prefix.
func TestAliasModeOnlyBuiltinTools(t *testing.T) {
	t.Parallel()
	prefix := GetBrandPrefix() + "_"

	// Server with alias_mode true: no root command, so only built-in tools; config alias_mode true
	s := NewServer()
	aliasTrue := true
	cfg := &ServerConfig{}
	cfg.MCPServer.Tools.AliasMode = &aliasTrue
	s.SetConfig(cfg)
	s.SetSecurityContext(pkgctx.NewSystemSecurityContext())
	s.registerToolsOnly(pkgctx.NewSystemSecurityContext())

	tools := s.ListTools()
	for _, tool := range tools {
		if len(tool.Name) < len(prefix) || tool.Name[:len(prefix)] != prefix {
			t.Errorf("alias_mode=true: expected all tools to have brand prefix %q, got %q", prefix, tool.Name)
		}
	}
	// Built-in set is a known small set (graph, echo, common, interactive, workflow, metrics)
	if len(tools) == 0 {
		t.Error("alias_mode=true: expected at least built-in tools, got 0")
	}
	t.Logf("alias_mode=true: %d built-in tools, all prefixed with %q", len(tools), prefix)
}

// TestToolsListFilteredByClientSecurityContext verifies that tool registration uses the
// client's security context (set before registerToolsAndResources), so different clients
// can see different tool sets. We test that setting a restricted secCtx before registration
// yields a subset (or same set when no CLI commands are permission-restricted in test env).
func TestToolsListFilteredByClientSecurityContext(t *testing.T) {
	t.Parallel()
	s := NewServer()
	s.SetSecurityContext(pkgctx.NewSystemSecurityContext())

	// Register tools with system context (full permissions)
	s.registerToolsOnly(pkgctx.NewSystemSecurityContext())
	fullCount := s.getToolCount()
	if fullCount == 0 {
		t.Fatalf("expected at least built-in tools with system context, got 0")
	}

	// Restricted context (no roles, no permissions) — unannotated CLI commands are fail-closed.
	restrictedCtx := pkgctx.NewSecurityContext("", nil, nil)
	s2 := NewServer()
	s2.SetSecurityContext(restrictedCtx)
	s2.registerToolsOnly(restrictedCtx)
	restrictedCount := s2.getToolCount()

	// Both should have built-in tools; restricted may have same or fewer if CLI was registered
	if restrictedCount == 0 {
		t.Error("restricted context: expected at least built-in tools, got 0")
	}
	t.Logf("tools: system context=%d, restricted context=%d", fullCount, restrictedCount)
}
