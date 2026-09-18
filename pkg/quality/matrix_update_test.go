package quality

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestUpdateMatrixCSVRow_codebaseVetting(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "m.csv")
	profPath := filepath.Join(dir, "profile.yaml")
	if err := fileutil.WriteSecureFile(profPath, []byte(`completion:
  gate_columns:
    - fully_vetted
  done_values:
    - yes
    - "no"
    - na
    - pending
`)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(csvPath, []byte("file_path,fully_vetted,cvs_id\npkg/a.go,pending,\npkg/b.go,no,\n")); err != nil {
		t.Fatal(err)
	}

	res, err := UpdateMatrixCSVRow(csvPath, profPath, "test", "file_path", "pkg/b.go", []string{"fully_vetted=yes"}, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Wrote {
		t.Fatal("expected wrote")
	}
	data, err := fileutil.ReadFile(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "pkg/b.go,yes") {
		t.Fatalf("expected updated row, got:\n%s", data)
	}
}

func TestUpdateMatrixCSVRow_dryRun(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "m.csv")
	profPath := filepath.Join(dir, "profile.yaml")
	if err := fileutil.WriteSecureFile(profPath, []byte(`completion:
  gate_columns:
    - fully_vetted
  done_values:
    - yes
    - "no"
    - na
    - pending
`)); err != nil {
		t.Fatal(err)
	}
	orig := "file_path,fully_vetted\nx.go,pending\n"
	if err := fileutil.WriteSecureFile(csvPath, []byte(orig)); err != nil {
		t.Fatal(err)
	}

	res, err := UpdateMatrixCSVRow(csvPath, profPath, "test", "file_path", "x.go", []string{"fully_vetted=na"}, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Wrote {
		t.Fatal("dry-run should not write")
	}
	if res.RowAfter["fully_vetted"] != "na" || res.RowAfter[objects.FieldKeyFilePath] != "x.go" {
		t.Fatalf("row_after preview: %#v", res.RowAfter)
	}
	data, _ := fileutil.ReadFile(csvPath)
	if string(data) != orig {
		t.Fatal("file should be unchanged")
	}
}

func TestUpdateMatrixCSVRow_valueMap(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "m.csv")
	profPath := filepath.Join(dir, "profile.yaml")
	if err := fileutil.WriteSecureFile(profPath, []byte(`completion:
  gate_columns:
    - fully_vetted
  done_values:
    - yes
    - pending
`)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(csvPath, []byte("file_path,fully_vetted\nx.go,pending\n")); err != nil {
		t.Fatal(err)
	}

	vm := map[string]string{"done": "yes"}
	res, err := UpdateMatrixCSVRow(csvPath, profPath, "test", "file_path", "x.go", []string{"fully_vetted=done"}, vm, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Updates["fully_vetted"] != "yes" {
		t.Fatalf("expected canonical yes, got %#v", res.Updates)
	}
	data, err := fileutil.ReadFile(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "x.go,yes") {
		t.Fatalf("expected value_map applied: %s", data)
	}
}

func TestUpdateMatrixCSVRow_backup(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "m.csv")
	profPath := filepath.Join(dir, "profile.yaml")
	if err := fileutil.WriteSecureFile(profPath, []byte(`completion:
  gate_columns:
    - fully_vetted
  done_values:
    - yes
    - pending
`)); err != nil {
		t.Fatal(err)
	}
	orig := "file_path,fully_vetted\nx.go,pending\n"
	if err := fileutil.WriteSecureFile(csvPath, []byte(orig)); err != nil {
		t.Fatal(err)
	}

	res, err := UpdateMatrixCSVRow(csvPath, profPath, "test", "file_path", "x.go", []string{"fully_vetted=yes"}, nil, false, &MatrixWriteOpts{Backup: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.BackupPath != csvPath+".bak" {
		t.Fatalf("backup path: %q", res.BackupPath)
	}
	bak, err := fileutil.ReadFile(csvPath + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if string(bak) != orig {
		t.Fatalf("backup should match pre-update file")
	}
}

func TestUpdateMatrixCSVByFilter_bulk(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "m.csv")
	profPath := filepath.Join(dir, "profile.yaml")
	if err := fileutil.WriteSecureFile(profPath, []byte(`completion:
  gate_columns:
    - fully_vetted
  done_values:
    - yes
    - "no"
    - na
    - pending
`)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(csvPath, []byte("file_path,fully_vetted\npkg/a.go,pending\npkg/b.go,pending\n")); err != nil {
		t.Fatal(err)
	}

	res, err := UpdateMatrixCSVByFilter(csvPath, profPath, "test", map[string]string{"fully_vetted": "pending"}, []string{"fully_vetted=yes"}, nil, 0, false, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.UpdatedCount != 2 || len(res.RowIndices) != 2 {
		t.Fatalf("got %+v", res)
	}
	if len(res.RowAfterSamples) != 2 {
		t.Fatalf("row_after_samples: %d", len(res.RowAfterSamples))
	}
	data, err := fileutil.ReadFile(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, "pkg/a.go,yes") || !strings.Contains(s, "pkg/b.go,yes") {
		t.Fatalf("expected both rows updated:\n%s", data)
	}
}

func TestUpdateMatrixCSVByFilter_cvsIDs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "m.csv")
	profPath := filepath.Join(dir, "profile.yaml")
	if err := fileutil.WriteSecureFile(profPath, []byte(`completion:
  gate_columns:
    - fully_vetted
  done_values:
    - yes
    - pending
`)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(csvPath, []byte("file_path,fully_vetted,cvs_id\npkg/a.go,pending,CVS-1\npkg/b.go,pending,CVS-1\npkg/c.go,pending,CVS-2\n")); err != nil {
		t.Fatal(err)
	}

	res, err := UpdateMatrixCSVByFilter(csvPath, profPath, "test", map[string]string{"fully_vetted": "pending"}, []string{"fully_vetted=yes"}, nil, 0, false, nil, "cvs_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.CvsIDs) != 2 || res.CvsIDs[0] != "CVS-1" || res.CvsIDs[1] != "CVS-2" {
		t.Fatalf("cvs_ids: %#v", res.CvsIDs)
	}
}

func TestUpdateMatrixCSVByFilter_limit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "m.csv")
	profPath := filepath.Join(dir, "profile.yaml")
	if err := fileutil.WriteSecureFile(profPath, []byte(`completion:
  gate_columns:
    - fully_vetted
  done_values:
    - yes
    - pending
`)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(csvPath, []byte("file_path,fully_vetted\na.go,pending\nb.go,pending\nc.go,pending\n")); err != nil {
		t.Fatal(err)
	}

	res, err := UpdateMatrixCSVByFilter(csvPath, profPath, "t", map[string]string{"fully_vetted": "pending"}, []string{"fully_vetted=yes"}, nil, 2, false, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.UpdatedCount != 2 || res.RowIndices[0] != 0 || res.RowIndices[1] != 1 {
		t.Fatalf("got %+v", res)
	}
	data, _ := fileutil.ReadFile(csvPath)
	if strings.Count(string(data), ",yes") != 2 {
		t.Fatalf("want 2 yes rows, got:\n%s", data)
	}
}

func TestUpdateMatrixCSVRow_invalidGate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "m.csv")
	profPath := filepath.Join(dir, "profile.yaml")
	if err := fileutil.WriteSecureFile(profPath, []byte(`completion:
  gate_columns:
    - fully_vetted
  done_values:
    - yes
    - "no"
`)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(csvPath, []byte("file_path,fully_vetted\nx.go,pending\n")); err != nil {
		t.Fatal(err)
	}

	_, err := UpdateMatrixCSVRow(csvPath, profPath, "test", "file_path", "x.go", []string{"fully_vetted=maybe"}, nil, false, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}
