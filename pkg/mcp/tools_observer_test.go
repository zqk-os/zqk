package mcp

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestHandleObserverSearch_findsSymbol(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "pkg", "demo", "demo.go")
	if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	src := "package demo\n\nfunc UniqueKey() string { return \"\" }\n"
	if err := fileutil.WriteFile(path, []byte(src), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	got, err := HandleObserverSearch(context.Background(), root, map[string]any{
		objects.FieldKeyName: "UniqueKey",
	})
	if err != nil {
		t.Fatal(err)
	}
	text, _ := got.(string)
	if !strings.Contains(text, "UniqueKey") || !strings.Contains(text, "pkg/demo/demo.go") {
		t.Fatalf("got %q", text)
	}
}

func TestHandleObserverSearch_requiresArg(t *testing.T) {
	t.Parallel()
	if _, err := HandleObserverSearch(context.Background(), t.TempDir(), map[string]any{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestRegisterObserverTools_exposesSearch(t *testing.T) {
	t.Parallel()
	s := NewServer()
	RegisterObserverTools(s)
	name := GetToolName(observerSearchToolSuffix)
	if _, ok := s.tools[name]; !ok {
		t.Fatalf("missing %s", name)
	}
}

func TestHandleObserverRegisterTool(t *testing.T) {
	s := NewServer()
	s.config = &ServerConfig{}

	args := map[string]any{
		"account_id": "ACC-12345",
		"roles":      []any{"admin"},
		"profile":    "mcp",
	}

	got, err := s.handleObserverRegisterTool(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}

	text, _ := got.(string)
	if !strings.Contains(text, "Successfully registered agent ACC-12345") {
		t.Fatalf("got %q", text)
	}

	if cfg, ok := s.config.MCPServer.Security.AgentRegistry["ACC-12345"]; !ok {
		t.Fatalf("agent not registered in config")
	} else if cfg.Profile != "mcp" {
		t.Fatalf("expected profile mcp, got %s", cfg.Profile)
	}
}

func TestRegisterObserverTools_exposesRegister(t *testing.T) {
	t.Parallel()
	s := NewServer()
	RegisterObserverTools(s)
	name := GetToolName(observerRegisterToolSuffix)
	if _, ok := s.tools[name]; !ok {
		t.Fatalf("missing %s", name)
	}
}
