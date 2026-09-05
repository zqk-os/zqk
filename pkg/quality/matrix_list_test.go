package quality

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestListMatricesFromRegistry(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := filepath.Join(dir, "matrix_registry.yaml")
	if err := fileutil.WriteSecureFile(p, []byte(`schema_version: 1
default_name: beta
matrices:
  alpha:
    description: "first"
    csv: a.csv
    profile: p.yaml
  beta:
    csv: b.csv
    profile: q.yaml
    session_ref_column: cvs_id
    value_map:
      x: "yes"
`)); err != nil {
		t.Fatal(err)
	}
	res, err := ListMatricesFromRegistry(dir, "matrix_registry.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if res.DefaultName != "beta" || len(res.Matrices) != 2 {
		t.Fatalf("got default=%q n=%d", res.DefaultName, len(res.Matrices))
	}
	if res.Matrices[0].Name != "alpha" || res.Matrices[1].Name != "beta" {
		t.Fatalf("order: %#v", res.Matrices)
	}
	if res.Matrices[1].SessionRefColumn != "cvs_id" || res.Matrices[1].ValueMapKeys != 1 {
		t.Fatalf("beta: %#v", res.Matrices[1])
	}
}
