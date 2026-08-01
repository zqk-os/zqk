package quality

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/utils/fileutil"
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
