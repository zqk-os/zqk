package crud

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"gopkg.in/yaml.v3"
)

func TestRewriteObjectIDForRawRename(t *testing.T) {
	t.Parallel()

	got, err := RewriteObjectIDForRawRename([]byte("id: GLO-1\nkind: glossary_term\ntitle: term\n"), "GLS-1")
	if err != nil {
		t.Fatal(err)
	}

	var obj map[string]any
	if err := yaml.Unmarshal(got, &obj); err != nil {
		t.Fatal(err)
	}
	if id := objects.GetString(obj, objects.FieldKeyID); id != "GLS-1" {
		t.Fatalf("id = %q, want GLS-1", id)
	}
	if title := objects.GetString(obj, objects.FieldKeyTitle); title != "term" {
		t.Fatalf("title = %q, want term", title)
	}
}

func TestRewriteObjectIDForRawRenameRejectsEmptyID(t *testing.T) {
	t.Parallel()

	if _, err := RewriteObjectIDForRawRename([]byte("id: GLS-1\nkind: glossary_term\n"), " "); err == nil {
		t.Fatal("expected empty ID error")
	}
}
