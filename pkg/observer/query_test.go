package observer

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestSearch_nameFindsTypeNotVendor(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeGo(t, filepath.Join(root, "pkg", "scheduler", "job.go"),
		"package scheduler\n\ntype BranchRef struct{ ID string }\nfunc Other() {}\n")
	writeGo(t, filepath.Join(root, "vendor", "fake", "job.go"),
		"package fake\n\ntype BranchRef struct{}\n")

	got, err := Search(context.Background(), Query{Root: root, Name: "BranchRef", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("hits = %#v, want exactly the pkg/scheduler type", got)
	}
	if got[0].Name != "BranchRef" || !strings.Contains(got[0].File, "pkg/scheduler/job.go") {
		t.Fatalf("hit = %#v", got[0])
	}
}

func TestSearch_pathListsFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeGo(t, filepath.Join(root, "pkg", "foo", "a.go"),
		"package foo\n\nfunc Alpha() {}\ntype Beta struct{}\n")

	got, err := Search(context.Background(), Query{Root: root, Path: "pkg/foo/a.go"})
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, e := range got {
		names[e.Name] = true
	}
	if !names["Alpha"] || !names["Beta"] {
		t.Fatalf("entities = %#v", got)
	}
}

func TestSearch_requiresNameOrPath(t *testing.T) {
	t.Parallel()
	if _, err := Search(context.Background(), Query{Root: t.TempDir()}); err == nil {
		t.Fatal("expected error")
	}
}

func TestSearch_rejectsEscape(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if _, err := Search(context.Background(), Query{Root: root, Path: "../etc"}); err == nil {
		t.Fatal("expected escape error")
	}
}

func TestSearchTokens_andInferPath(t *testing.T) {
	t.Parallel()
	toks := SearchTokens("Add BranchRef uniqueness check to pkg/scheduler job steward")
	joined := strings.Join(toks, " ")
	if !strings.Contains(joined, "branchref") || !strings.Contains(joined, "scheduler") {
		t.Fatalf("tokens = %v", toks)
	}
	if InferSourcePath("touch pkg/scheduler/job.go now") != "pkg/scheduler/job.go" {
		t.Fatal(InferSourcePath("touch pkg/scheduler/job.go now"))
	}
}

func TestFormatHit(t *testing.T) {
	t.Parallel()
	line := FormatHit(Entity{Name: "Foo", Kind: "function", File: "pkg/x.go", Line: 3, Signature: "func Foo()"})
	if !strings.Contains(line, "`pkg/x.go:3`") || !strings.Contains(line, "Foo") {
		t.Fatal(line)
	}
}

func writeGo(t *testing.T, path, src string) {
	t.Helper()
	if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(path, []byte(src), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
}
