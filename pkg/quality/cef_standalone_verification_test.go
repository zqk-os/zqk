package quality

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type cefScorecard struct {
	CEFVersion      string                 `json:"cef_version"`
	AssessedAt      string                 `json:"assessed_at"`
	RepoFingerprint string                 `json:"repo_fingerprint"`
	Integrator      string                 `json:"integrator"`
	Notes           string                 `json:"notes"`
	EnvelopeMin     float64                `json:"envelope_min"`
	Axes            map[string]cefAxisData `json:"axes"`
}

type cefAxisData struct {
	Grade      float64  `json:"grade"`
	Confidence float64  `json:"confidence"`
	Drivers    []string `json:"drivers"`
	Notes      string   `json:"notes"`
}

func TestCEFStandaloneLaunchMatrixAndScorecard(t *testing.T) {
	// Find project root
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root := filepath.Clean(filepath.Join(wd, "../.."))

	// 1. Validate the live repository matrix registry
	res, err := ValidateMatrixRegistry(root, "cef_diamond_scorecard", "docs/quality/matrix_registry.yaml")
	if err != nil {
		t.Fatalf("ValidateMatrixRegistry: %v", err)
	}
	if !res.AllOK() {
		t.Fatalf("matrix registry not OK: %#v", res.Entries)
	}

	mRes, err := ResolveMatrixForCLI(root, "cef_diamond_scorecard", "docs/quality/matrix_registry.yaml", "", "")
	if err != nil {
		t.Fatalf("ResolveMatrixForCLI: %v", err)
	}

	prof, doneVals, err := LoadMatrixProfileYAML(mRes.ProfilePath)
	if err != nil {
		t.Fatalf("LoadMatrixProfileYAML: %v", err)
	}

	summary, err := SummarizeMatrixCSV(mRes.CSVPath, prof, doneVals, false, mRes.SessionRefColumn)
	if err != nil {
		t.Fatalf("SummarizeMatrixCSV: %v", err)
	}

	if summary.RowTotal < 18 {
		t.Errorf("expected at least 18 rows in CEF matrix, got %d", summary.RowTotal)
	}
	if summary.FullyDoneRows != summary.RowTotal {
		t.Errorf("expected all rows fully done, got %d/%d", summary.FullyDoneRows, summary.RowTotal)
	}

	// 2. Validate the standalone launch scorecard JSON
	scorecardPath := filepath.Join(root, "docs/quality/cef-runs/2026-09-24-CORE-STANDALONE/scorecard.json")
	data, err := fileutil.ReadFile(scorecardPath)
	if err != nil {
		t.Fatalf("read standalone scorecard: %v", err)
	}

	var sc cefScorecard
	if err := json.Unmarshal(data, &sc); err != nil {
		t.Fatalf("unmarshal scorecard: %v", err)
	}

	if sc.CEFVersion != "0.1.0" {
		t.Errorf("expected CEF version 0.1.0, got %q", sc.CEFVersion)
	}
	if sc.EnvelopeMin < 4.0 {
		t.Errorf("expected envelope_min >= 4.0, got %f", sc.EnvelopeMin)
	}

	requiredAxes := []string{"RDB", "MNT", "TST", "REL", "OBS", "RCV", "SEC", "ROB"}
	for _, axis := range requiredAxes {
		entry, ok := sc.Axes[axis]
		if !ok {
			t.Errorf("missing required diamond axis %s", axis)
			continue
		}
		if entry.Grade < 4.0 {
			t.Errorf("axis %s grade %f is below floor 4.0", axis, entry.Grade)
		}
		if entry.Confidence <= 0 {
			t.Errorf("axis %s confidence must be positive, got %f", axis, entry.Confidence)
		}
	}
}
