// BLI-STARTER-COMMUNITY-032 / PRI-STARTER-COMMUNITY-032 coverage elevation
package adapters

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestAgentTranslator_Translate_SkillsYmlAndErrors(t *testing.T) {
	t.Parallel()
	tr := NewAgentTranslator()
	ctx := context.Background()
	obj, err := tr.Translate(ctx, []byte("skills:\n  - a\n"), "yml", "pack.yml")
	if err != nil {
		t.Fatal(err)
	}
	if obj[objects.FieldKeyKind] != "tool_spec" || obj[objects.FieldKeyID] != "ext-pack" {
		t.Fatalf("%v", obj)
	}
	kept, err := tr.Translate(ctx, []byte(`{"kind":"policy","id":"keep-me"}`), "json", "x.json")
	if err != nil {
		t.Fatal(err)
	}
	if kept[objects.FieldKeyKind] != "policy" || kept[objects.FieldKeyID] != "keep-me" {
		t.Fatalf("%v", kept)
	}
	fb, err := tr.Translate(ctx, []byte("{}"), "json", "plain.json")
	if err != nil || fb[objects.FieldKeyKind] != "tool_spec" {
		t.Fatalf("%v %v", fb, err)
	}
	if _, err := tr.Translate(ctx, []byte("{"), "json", "bad.json"); err == nil {
		t.Fatal("bad json")
	}
	if _, err := tr.Translate(ctx, []byte(":\n-"), "yaml", "bad.yaml"); err == nil {
		t.Fatal("bad yaml")
	}
}

func TestGeminiAdapter_Ingest_FileNotDirAndBadJSON(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	filePath := filepath.Join(root, "not-a-dir")
	if err := fileutil.WriteFile(filePath, []byte("x"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	a := NewGeminiAdapter(logging.GetLoggerFromProfile("test"))
	if err := a.Ingest(context.Background(), filePath); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := fileutil.WriteFile(filepath.Join(dir, "broken.json"), []byte("{"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := a.Ingest(context.Background(), dir); err == nil {
		t.Fatal("bad json ingest")
	}
}
