// BLI-STARTER-COMMUNITY-060 / PRI-STARTER-COMMUNITY-060 coverage elevation
package parser

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestExtraParseExtractRefsAndEmbedding(t *testing.T) {
	p := NewYAMLParser()
	if _, err := p.ParseFile(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Fatal("expected missing file")
	}
	bad := filepath.Join(t.TempDir(), "bad.yaml")
	if err := fileutil.WriteFile(bad, []byte(":\n  -"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ParseFile(bad); err == nil {
		t.Fatal("expected yaml error")
	}
	good := filepath.Join(t.TempDir(), "obj.yaml")
	body := "id: BLI-1\nkind: backlog_item\ntitle: Hello\nstatus: planned\ncontext: ctx\ndescription: desc\nmilestone_ref: MIL-1\nbranch_ref: leftover\nnested:\n  goal_ref: GOAL-1\n  extra: 1\nitems:\n  - requirement_ref: REQ-1\n  - {requirement_ref: REQ-2, name: x}\ncounts: 3\n"
	if err := fileutil.WriteFile(good, []byte(body), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	obj, err := p.ParseFile(good)
	if err != nil {
		t.Fatal(err)
	}
	if obj.ID != "BLI-1" || obj.Kind != "backlog_item" {
		t.Fatalf("parsed %+v", obj)
	}
	if _, err := p.ParseBytes([]byte(body)); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ParseBytes([]byte("{")); err == nil {
		t.Fatal("expected parse bytes error")
	}

	refs := p.ExtractReferenceFields(obj.Properties)
	if refs["milestone_ref"] == nil || refs["nested.goal_ref"] == nil {
		t.Fatalf("refs=%v", refs)
	}
	if _, ok := refs["branch_ref"]; ok {
		t.Fatalf("leftover branch_ref should be skipped: %v", refs)
	}
	nestedIface := map[any]any{"doc_entry_ref": "DOC-1", 1: "skip"}
	sliceIface := []any{
		map[any]any{"test_case_ref": "TC-1"},
		"not-a-map",
	}
	more := p.ExtractReferenceFields(map[string]any{
		"outer":                     nestedIface,
		"list":                      sliceIface,
		"plain":                     "x",
		objects.FieldKeyContext:     "ctx2",
		objects.FieldKeyDescription: "d2",
	})
	_ = more
	text := p.ExtractEmbeddingText(obj)
	if text == "" {
		t.Fatal("expected embedding text")
	}
	empty := p.ExtractEmbeddingText(&ParsedObject{})
	if empty != "" {
		t.Fatalf("expected empty embedding, got %q", empty)
	}
	_ = getString(map[string]any{"id": 7, "kind": "x"}, "id")
	_ = getString(nil, "id")
}
