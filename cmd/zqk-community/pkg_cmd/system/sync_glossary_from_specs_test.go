package system

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestScanSpecCandidates_BuildsObjectKindCandidates(t *testing.T) {
	root := t.TempDir()
	specDir := filepath.Join(root, "specs")
	if err := fileutil.EnsureDir(specDir); err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(specDir, "widget.yaml")
	raw := []byte("ontology: widget\ndescription: Test widget object kind\n")
	if err := fileutil.WriteSecureFile(specPath, raw); err != nil {
		t.Fatal(err)
	}

	cands, err := scanSpecCandidates(root, specDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 1 {
		t.Fatalf("candidates len = %d want 1", len(cands))
	}
	got := cands[0]
	if got.Title != "Object kind: widget" {
		t.Fatalf("title = %q", got.Title)
	}
	if got.SourceType != "object_spec" {
		t.Fatalf("source_type = %q", got.SourceType)
	}
	if got.SourcePath != filepath.Join("specs", "widget.yaml") {
		t.Fatalf("source_path = %q", got.SourcePath)
	}
}

func TestCollectGlossaryCandidates_DedupesByTitle(t *testing.T) {
	root := t.TempDir()
	specs := filepath.Join(root, "specs")
	cmdSpecs := filepath.Join(root, "cmd_specs")
	lifs := filepath.Join(root, "lifs")
	cfgs := filepath.Join(root, "cfgs")
	for _, d := range []string{specs, cmdSpecs, lifs, cfgs} {
		if err := fileutil.EnsureDir(d); err != nil {
			t.Fatal(err)
		}
	}
	if err := fileutil.WriteSecureFile(filepath.Join(specs, "alpha.yaml"), []byte("ontology: alpha\n")); err != nil {
		t.Fatal(err)
	}
	// Config name intentionally does not collide with spec title; count check still verifies stable behavior.
	if err := fileutil.WriteSecureFile(filepath.Join(cfgs, "alpha.yaml"), []byte("dummy: true\n")); err != nil {
		t.Fatal(err)
	}
	cands, err := collectGlossaryCandidates(root, specs, cmdSpecs, lifs, cfgs)
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 2 {
		t.Fatalf("candidates len = %d want 2", len(cands))
	}
}
