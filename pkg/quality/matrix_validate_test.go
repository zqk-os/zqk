package quality

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestValidateMatrixRegistry_ok(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	regPath := filepath.Join(dir, "matrix_registry.yaml")
	profPath := filepath.Join(dir, "p.yaml")
	csvPath := filepath.Join(dir, "m.csv")
	if err := fileutil.WriteSecureFile(profPath, []byte(`completion:
  gate_columns:
    - fully_vetted
  done_values:
    - yes
    - pending
`)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(csvPath, []byte("file_path,fully_vetted,cvs_id\na.go,pending,\n")); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(regPath, []byte(`schema_version: 1
default_name: one
matrices:
  one:
    csv: m.csv
    profile: p.yaml
    session_ref_column: cvs_id
`)); err != nil {
		t.Fatal(err)
	}

	res, err := ValidateMatrixRegistry(dir, "", "matrix_registry.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !res.AllOK() || len(res.Entries) != 1 {
		t.Fatalf("AllOK=%v entries=%#v", res.AllOK(), res.Entries)
	}
	if res.Entries[0].RowCount != 1 {
		t.Fatalf("row count %d", res.Entries[0].RowCount)
	}
}

func TestValidateMatrixRegistry_missingSessionColumn(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	regPath := filepath.Join(dir, "matrix_registry.yaml")
	profPath := filepath.Join(dir, "p.yaml")
	csvPath := filepath.Join(dir, "m.csv")
	if err := fileutil.WriteSecureFile(profPath, []byte(`completion:
  gate_columns:
    - fully_vetted
  done_values:
    - yes
`)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(csvPath, []byte("file_path,fully_vetted\na.go,pending\n")); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(regPath, []byte(`schema_version: 1
default_name: one
matrices:
  one:
    csv: m.csv
    profile: p.yaml
    session_ref_column: cvs_id
`)); err != nil {
		t.Fatal(err)
	}

	res, err := ValidateMatrixRegistry(dir, "", "matrix_registry.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if res.AllOK() {
		t.Fatal("expected failure")
	}
}
