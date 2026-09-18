package quality

import (
	"encoding/csv"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
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

func TestFeaturesTraceabilityMatrix100PercentDelivered(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("failed to determine repo root: %v", err)
	}
	csvPath := filepath.Join(root, "docs", "quality", "features_traceability.csv")
	if _, err := fileutil.Stat(csvPath); err != nil {
		t.Skipf("skipping test; features_traceability.csv not found at %s", csvPath)
	}

	f, err := fileutil.Open(csvPath)
	if err != nil {
		t.Fatalf("failed to open csv: %v", err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		t.Fatalf("failed to read header: %v", err)
	}
	colMap := make(map[string]int)
	for i, h := range header {
		colMap[strings.TrimSpace(h)] = i
	}
	for _, reqCol := range []string{"Requirement ID", "Delivered", "Test Cases", "Verification Hash"} {
		if _, ok := colMap[reqCol]; !ok {
			t.Fatalf("missing required column %q", reqCol)
		}
	}

	reqIdx := colMap["Requirement ID"]
	delivIdx := colMap["Delivered"]
	testIdx := colMap["Test Cases"]
	hashIdx := colMap["Verification Hash"]

	rowCount := 0
	hex64Regex := regexp.MustCompile(`^[a-fA-F0-9]{64}$`)

	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("row %d read error: %v", rowCount+1, err)
		}
		rowCount++
		reqID := strings.TrimSpace(rec[reqIdx])
		delivered := strings.ToLower(strings.TrimSpace(rec[delivIdx]))
		testCases := strings.TrimSpace(rec[testIdx])
		vHash := strings.TrimSpace(rec[hashIdx])

		if delivered != "yes" && delivered != "y" {
			t.Errorf("row %d (%s): Delivered is %q, expected 'yes'", rowCount, reqID, delivered)
		}
		if testCases == "" {
			t.Errorf("row %d (%s): Test Cases is empty", rowCount, reqID)
		}
		if !hex64Regex.MatchString(vHash) {
			t.Errorf("row %d (%s): Verification Hash %q is not a valid 64-character hex sha256", rowCount, reqID, vHash)
		}
	}

	if rowCount == 0 {
		t.Fatalf("features_traceability.csv has 0 data rows")
	}
}
