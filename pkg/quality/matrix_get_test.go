package quality

import (
	"bytes"
	"encoding/csv"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestQueryMatrixCSV_filters(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "m.csv")
	if err := fileutil.WriteSecureFile(csvPath, []byte("file_path,fully_vetted\na.go,yes\nb.go,pending\n")); err != nil {
		t.Fatal(err)
	}
	res, err := QueryMatrixCSV(csvPath, "/dev/null", "t", map[string]string{"fully_vetted": "pending"}, "", false, "", "", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.RowCount != 1 || res.Rows[0][objects.FieldKeyFilePath] != "b.go" {
		t.Fatalf("got %+v", res.Rows)
	}
}

func TestQueryMatrixCSV_goOnly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "m.csv")
	if err := fileutil.WriteSecureFile(csvPath, []byte("file_path,fully_vetted\na.go,yes\nb.md,pending\n")); err != nil {
		t.Fatal(err)
	}
	res, err := QueryMatrixCSV(csvPath, "/dev/null", "t", nil, "", true, "", "", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.RowCount != 1 || res.Rows[0][objects.FieldKeyFilePath] != "a.go" {
		t.Fatalf("got %+v", res.Rows)
	}
}

func TestQueryMatrixCSV_limit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "m.csv")
	if err := fileutil.WriteSecureFile(csvPath, []byte("file_path\na.go\nb.go\n")); err != nil {
		t.Fatal(err)
	}
	res, err := QueryMatrixCSV(csvPath, "/dev/null", "t", nil, "", false, "", "", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.RowCount != 1 {
		t.Fatalf("want 1 row, got %d", res.RowCount)
	}
}

func TestQueryMatrixCSV_cvsIDRequiresColumn(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "m.csv")
	if err := fileutil.WriteSecureFile(csvPath, []byte("file_path\na.go\n")); err != nil {
		t.Fatal(err)
	}
	_, err := QueryMatrixCSV(csvPath, "/dev/null", "t", nil, "", false, "CVS-1", "", 0, nil)
	if err == nil {
		t.Fatal("expected error when session ref column missing")
	}
}

func TestQueryMatrixCSV_columnsProjection(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "m.csv")
	if err := fileutil.WriteSecureFile(csvPath, []byte("file_path,fully_vetted,notes\na.go,yes,n1\nb.go,pending,n2\n")); err != nil {
		t.Fatal(err)
	}
	res, err := QueryMatrixCSV(csvPath, "/dev/null", "t", nil, "", false, "", "", 0, []string{"fully_vetted", "file_path"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Header) != 2 || res.Header[0] != "fully_vetted" || res.Header[1] != "file_path" {
		t.Fatalf("header: %#v", res.Header)
	}
	if res.RowCount != 2 {
		t.Fatalf("rows: %d", res.RowCount)
	}
	if _, ok := res.Rows[0][objects.FieldKeyNotes]; ok {
		t.Fatal("expected notes omitted")
	}
	if res.Rows[0]["fully_vetted"] != "yes" || res.Rows[0][objects.FieldKeyFilePath] != "a.go" {
		t.Fatalf("row0: %#v", res.Rows[0])
	}
}

func TestQueryMatrixCSV_columnsUnknown(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "m.csv")
	if err := fileutil.WriteSecureFile(csvPath, []byte("file_path\na.go\n")); err != nil {
		t.Fatal(err)
	}
	_, err := QueryMatrixCSV(csvPath, "/dev/null", "t", nil, "", false, "", "", 0, []string{"nope"})
	if err == nil {
		t.Fatal("expected unknown column error")
	}
}

func TestFormatMatrixGetResultCSV(t *testing.T) {
	t.Parallel()
	res := &MatrixGetResult{
		Header: []string{"a", "b"},
		Rows: []map[string]string{
			{"a": "1", "b": "x"},
			{"a": "2", "b": "y,z"},
		},
		RowCount: 2,
	}
	out, err := FormatMatrixGetResultCSV(res)
	if err != nil {
		t.Fatal(err)
	}
	r := csv.NewReader(strings.NewReader(string(out)))
	recs, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 || recs[0][0] != "a" || recs[2][1] != "y,z" {
		t.Fatalf("records: %#v", recs)
	}
	if !bytes.HasSuffix(out, []byte("\n")) {
		t.Fatal("expected trailing newline")
	}
}

func TestQueryMatrixCSV_columnsDuplicate(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "m.csv")
	if err := fileutil.WriteSecureFile(csvPath, []byte("a,b\n1,2\n")); err != nil {
		t.Fatal(err)
	}
	_, err := QueryMatrixCSV(csvPath, "/dev/null", "t", nil, "", false, "", "", 0, []string{"a", "a"})
	if err == nil {
		t.Fatal("expected duplicate column error")
	}
}
