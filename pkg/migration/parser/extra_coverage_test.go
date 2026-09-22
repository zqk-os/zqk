// BLI-STARTER-COMMUNITY-043 / PRI-STARTER-COMMUNITY-043 coverage elevation
package parser

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestExtraParseBytesNestedRefsAndEmbedding(t *testing.T) {
	p := NewYAMLParser()
	if _, err := p.ParseFile(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Fatal("missing file")
	}
	if _, err := p.ParseBytes([]byte("{not yaml")); err == nil {
		t.Fatal("bad bytes")
	}
	obj, err := p.ParseBytes([]byte("id: X-1\nkind: backlog_item\ntitle: Hello\nstatus: planned\n"))
	if err != nil || obj.ID != "X-1" || obj.Title != "Hello" {
		t.Fatalf("bytes = %#v %v", obj, err)
	}
	if getString(map[string]any{"id": 7}, "id") != "" || getString(map[string]any{}, "id") != "" {
		t.Fatal("getString")
	}

	nested := map[string]any{
		"meta": map[string]any{
			"owner_ref": "ACC-1",
			"inner":     map[interface{}]interface{}{"team_ref": "TEAM-1", 1: "skip"},
		},
		"items": []any{
			map[string]any{"child_ref": "CH-1"},
			map[interface{}]interface{}{"other_ref": "OT-1"},
			"skip",
		},
		objects.FieldKeyTitle: "nested",
	}
	refs := p.ExtractReferenceFields(nested)
	if refs["meta.owner_ref"] != "ACC-1" || refs["items[0].child_ref"] != "CH-1" {
		t.Fatalf("nested refs = %#v", refs)
	}
	if refs["meta.inner.team_ref"] != "TEAM-1" || refs["items[1].other_ref"] != "OT-1" {
		t.Fatalf("iface refs = %#v", refs)
	}

	embed := p.ExtractEmbeddingText(&ParsedObject{
		Title: "T",
		Properties: map[string]any{
			objects.FieldKeyContext:     "C",
			objects.FieldKeyDescription: "D",
		},
	})
	if embed != "T C D" {
		t.Fatalf("embed = %q", embed)
	}
	if p.ExtractEmbeddingText(&ParsedObject{}) != "" {
		t.Fatal("empty embed")
	}

	okPath := filepath.Join(t.TempDir(), "ok.yaml")
	if err := fileutil.WriteFile(okPath, []byte("id: 1\nkind: k\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ParseFile(okPath); err != nil {
		t.Fatal(err)
	}
}
