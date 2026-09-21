package vds

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestApplyIndependentVerifyYes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chunks.yaml")
	src := `# keep this comment
schema: zqk_vds_chunks_v1
chunks:
  - chunk_id: a
    stage: design
    claim: c
    rubric_ref: r
    dsl_checks: [path_exists:x]
    evidence_refs: [e]
    independent_verify: pending
  - chunk_id: b
    stage: design
    claim: c2
    rubric_ref: r
    dsl_checks: [path_exists:x]
    evidence_refs: [e]
    independent_verify: yes
`
	if err := fileutil.WriteFile(path, []byte(src), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	n, err := ApplyIndependentVerifyYes(path, []string{"a", "b", "missing"})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("updated=%d want 1", n)
	}
	raw, err := fileutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "# keep this comment") {
		t.Fatalf("surgical apply dropped comments:\n%s", raw)
	}
	chunks, err := LoadChunks(path)
	if err != nil {
		t.Fatal(err)
	}
	if chunks[0].IndependentVerify != "yes" {
		t.Fatalf("a=%q", chunks[0].IndependentVerify)
	}
}

func TestLoadChunks_reloadsWhenStampMoves(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chunks.yaml")
	write := func(claim string) {
		t.Helper()
		body := []byte(`schema: zqk_vds_chunks_v1
chunks:
  - chunk_id: a
    stage: design
    claim: ` + claim + `
    rubric_ref: r
    dsl_checks: [path_exists:x]
    evidence_refs: [e]
    independent_verify: pending
`)
		if err := fileutil.WriteFile(path, body, paths.FilePerm644); err != nil {
			t.Fatal(err)
		}
	}
	write("one")
	first, err := LoadChunks(path)
	if err != nil {
		t.Fatal(err)
	}
	if first[0].Claim != "one" {
		t.Fatalf("got %q", first[0].Claim)
	}
	write("two")
	later := time.Now().Add(2 * time.Second)
	if err := fileutil.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	second, err := LoadChunks(path)
	if err != nil {
		t.Fatal(err)
	}
	if second[0].Claim != "two" {
		t.Fatalf("expected reload after stamp move, got %q", second[0].Claim)
	}
}

func TestPredPathExists(t *testing.T) {
	dir := t.TempDir()
	if err := fileutil.WriteFile(filepath.Join(dir, "here.txt"), []byte("x"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	ok := EvalPredicate(t.Context(), "path_exists:here.txt", Chunk{}, EvalOptions{ProjectRoot: dir})
	if !ok.OK {
		t.Fatalf("%+v", ok)
	}
	miss := EvalPredicate(t.Context(), "path_exists:nope.txt", Chunk{}, EvalOptions{ProjectRoot: dir})
	if miss.OK {
		t.Fatal("expected miss")
	}
}
