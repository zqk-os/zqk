package quality

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestApplyValueMapToUpdates(t *testing.T) {
	t.Parallel()
	m := map[string]string{"a": "done", "b": "keep"}
	ApplyValueMapToUpdates(m, map[string]string{"done": "yes"})
	if m["a"] != "yes" || m["b"] != "keep" {
		t.Fatalf("got %#v", m)
	}
}

func TestLoadMatrixRegistry_valueMap(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := filepath.Join(dir, "matrix_registry.yaml")
	if err := fileutil.WriteSecureFile(p, []byte(`schema_version: 1
default_name: m
matrices:
  m:
    csv: a.csv
    profile: p.yaml
    value_map:
      Done: yes
`)); err != nil {
		t.Fatal(err)
	}
	reg, err := LoadMatrixRegistry(dir, "matrix_registry.yaml")
	if err != nil {
		t.Fatal(err)
	}
	e := reg.Matrices["m"]
	if e.ValueMap["Done"] != "yes" {
		t.Fatalf("yaml: %#v", e.ValueMap)
	}
	res, err := ResolveMatrixForCLI(dir, "m", "matrix_registry.yaml", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.ValueMap["done"] != "yes" {
		t.Fatalf("normalized: %#v", res.ValueMap)
	}
}

func TestLoadMatrixRegistry_missingDefaultAutoScaffolds(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	reg, err := LoadMatrixRegistry(dir, "")
	if err != nil {
		t.Fatalf("expected auto-scaffold without error, got: %v", err)
	}
	if reg == nil || len(reg.Matrices) != 0 {
		t.Fatalf("expected empty registry, got %#v", reg)
	}
	// Verify that the file was created on disk
	expectedPath := filepath.Join(dir, "docs", "quality", "matrix_registry.yaml")
	data, err := fileutil.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("expected registry file created at %s, got err: %v", expectedPath, err)
	}
	if len(data) == 0 {
		t.Fatalf("created file at %s is empty", expectedPath)
	}

	// Test listing empty registry
	listRes, err := ListMatricesFromRegistry(dir, "")
	if err != nil {
		t.Fatalf("ListMatricesFromRegistry: %v", err)
	}
	if len(listRes.Matrices) != 0 {
		t.Fatalf("expected 0 matrices, got %d", len(listRes.Matrices))
	}

	// Test resolving empty registry gives clear actionable message
	_, err = ResolveMatrixForCLI(dir, "", "", "", "")
	if err == nil {
		t.Fatal("expected error resolving empty registry")
	}
	if !strings.Contains(err.Error(), "no matrices configured") {
		t.Fatalf("expected 'no matrices configured' in error, got: %v", err)
	}
}
