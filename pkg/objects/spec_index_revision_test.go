package objects

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestBuildSpecIndexFromSpecsDir_SetsBuilderSpecCacheRevision(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	specsDir := filepath.Join(tmp, "object_specs")
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	specFile := filepath.Join(specsDir, "tiny_kind.yaml")
	content := `ontology: tiny_kind
schema_version: "2.0.0"
visibility: internal
fields:
  id:
    type: string
`
	if err := fileutil.WriteFile(specFile, []byte(content), paths.FilePerm644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	idx, err := BuildSpecIndexFromSpecsDir(specsDir)
	if err != nil {
		t.Fatalf("BuildSpecIndexFromSpecsDir: %v", err)
	}
	if idx == nil {
		t.Fatal("nil index")
	}
	if _, ok := idx.Kinds["tiny_kind"]; !ok {
		t.Fatal("expected tiny_kind in index")
	}
	// Fresh loader: no invalidation during build, revision stays 0.
	if idx.BuilderSpecCacheRevision != 0 {
		t.Fatalf("BuilderSpecCacheRevision want 0, got %d", idx.BuilderSpecCacheRevision)
	}
}

func TestKindNamesFromSpecIndex(t *testing.T) {
	t.Parallel()
	t.Run("nil index errors", func(t *testing.T) {
		t.Parallel()
		if _, err := KindNamesFromSpecIndex(nil); err == nil {
			t.Fatal("expected error for nil index")
		}
	})
	t.Run("empty kinds errors", func(t *testing.T) {
		t.Parallel()
		if _, err := KindNamesFromSpecIndex(&SpecIndex{Kinds: nil}); err == nil {
			t.Fatal("expected error when Kinds is nil")
		}
	})
	t.Run("matches index keys", func(t *testing.T) {
		t.Parallel()
		tmp := t.TempDir()
		specsDir := filepath.Join(tmp, "object_specs")
		if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		for _, name := range []string{"a_kind.yaml", "b_kind.yaml"} {
			k := strings.TrimSuffix(name, ".yaml")
			content := "ontology: " + k + "\nschema_version: \"2.0.0\"\nvisibility: internal\nfields: {}\n"
			if err := fileutil.WriteFile(filepath.Join(specsDir, name), []byte(content), paths.FilePerm644); err != nil {
				t.Fatalf("write %s: %v", name, err)
			}
		}
		idx, err := BuildSpecIndexFromSpecsDir(specsDir)
		if err != nil {
			t.Fatalf("BuildSpecIndexFromSpecsDir: %v", err)
		}
		m, err := KindNamesFromSpecIndex(idx)
		if err != nil {
			t.Fatalf("KindNamesFromSpecIndex: %v", err)
		}
		if len(m) != 2 {
			t.Fatalf("want 2 kinds, got %d", len(m))
		}
		if _, ok := m["a_kind"]; !ok {
			t.Fatal("missing a_kind")
		}
		if _, ok := m["b_kind"]; !ok {
			t.Fatal("missing b_kind")
		}
	})
}

func TestRefreshMaterializedSpecIndex_WritesSpecIndexJSON(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	specsDir := filepath.Join(tmp, paths.ProcessInternalObjectSpecsDir)
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir specs: %v", err)
	}
	content := `ontology: tiny_refresh
schema_version: "2.0.0"
visibility: internal
fields:
  id:
    type: string
`
	if err := fileutil.WriteFile(filepath.Join(specsDir, "tiny_refresh.yaml"), []byte(content), paths.FilePerm644); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	idx, err := RefreshMaterializedSpecIndex(tmp)
	if err != nil {
		t.Fatalf("RefreshMaterializedSpecIndex: %v", err)
	}
	if idx == nil || len(idx.Kinds) < 1 {
		t.Fatalf("expected kinds in index, got %#v", idx)
	}
	outPath := filepath.Join(tmp, paths.ProcessInternalDir, "spec_index.json")
	data, err := fileutil.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read spec_index.json: %v", err)
	}
	if !strings.Contains(string(data), "tiny_refresh") {
		t.Fatalf("spec_index.json should list ontology; got %.200q", string(data))
	}
}
