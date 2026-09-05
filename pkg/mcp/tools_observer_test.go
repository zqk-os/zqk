package mcp

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
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
