package agentprompt

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestObserverToolAccessSection_namesSearchTool(t *testing.T) {
	t.Parallel()
	got := ObserverToolAccessSection()
	if !strings.Contains(got, "observer_search") || !strings.Contains(got, "read_code") {
		t.Fatal(got)
	}
	if strings.Contains(got, "Top 30 Highest Complexity") {
		t.Fatal("access section must not dump a census")
	}
}

func TestRelevantObserverSection_includesHits(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "pkg", "steward", "key.go")
	if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	src := "package steward\n\ntype UniqueKey struct{ ID string }\n"
	if err := fileutil.WriteFile(path, []byte(src), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	got := RelevantObserverSection(context.Background(), root, "Add UniqueKey check", "")
	if !strings.Contains(got, "observer_search") {
		t.Fatal(got)
	}
	if !strings.Contains(got, "UniqueKey") || !strings.Contains(got, "pkg/steward/key.go") {
		t.Fatalf("expected task-relevant hit:\n%s", got)
	}
}
