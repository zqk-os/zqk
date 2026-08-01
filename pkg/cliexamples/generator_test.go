package cliexamples

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"gopkg.in/yaml.v3"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skip("go.mod not found")
		}
		dir = parent
	}
}

// TestGenerateYAMLExample_ParsesAsYAML ensures the `zqk new object` draft path emits valid YAML
// with core keys (same spec-driven source as object template).
func TestGenerateYAMLExample_ParsesAsYAML(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	specsDir := filepath.Join(datacell.CellCASPrimaryDir(root, "_internal"), "object_specs")
	if _, err := os.Stat(specsDir); err != nil {
		t.Skip("object_specs not present")
	}
	if _, err := os.Stat(filepath.Join(root, paths.ProjectDataDir)); err != nil {
		t.Skip(".zqk not present")
	}

	g, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	yamlStr, err := g.GenerateYAMLExample("goal", "")
	if err != nil {
		t.Fatalf("GenerateYAMLExample: %v", err)
	}
	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(yamlStr), &parsed); err != nil {
		t.Fatalf("not valid YAML: %v\n%s", err, yamlStr)
	}
	if got, _ := parsed[objects.FieldKeyKind].(string); got != "goal" {
		t.Fatalf("kind = %q, want goal", got)
	}
	if _, ok := parsed[objects.FieldKeySchemaVersion]; !ok {
		t.Fatal("missing schema_version")
	}
}

func TestGenerateObjectSpecKindDraft_InheritedDefaults(t *testing.T) {
	t.Parallel()
	root := repoRoot(t)
	specsDir := filepath.Join(datacell.CellCASPrimaryDir(root, "_internal"), "object_specs")
	if _, err := os.Stat(specsDir); err != nil {
		t.Skip("object_specs not present")
	}

	g, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	out, err := g.GenerateObjectSpecKindDraft("my_kind", "base_object")
	if err != nil {
		t.Fatalf("GenerateObjectSpecKindDraft: %v", err)
	}
	if !strings.Contains(out, "storage_profile: cas_entity") {
		t.Fatalf("expected inherited cas_entity from auditable chain; got:\n%s", out)
	}
	if !strings.Contains(out, "extends: base_object") {
		t.Fatal("missing extends")
	}
	if !strings.Contains(out, "Inheritance model") {
		t.Fatal("expected inheritance comment block")
	}

	streamOut, err := g.GenerateObjectSpecKindDraft("hv_kind", "base_metric")
	if err != nil {
		t.Fatalf("GenerateObjectSpecKindDraft base_metric: %v", err)
	}
	if !strings.Contains(streamOut, "storage_profile: stream") {
		t.Fatalf("expected stream profile from base_metric chain; got:\n%s", streamOut)
	}
}
