package quality

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/pipeline"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestNormalizeDoneValue(t *testing.T) {
	t.Parallel()

	if v := normalizeDoneValue(true); v != "yes" {
		t.Errorf("expected yes for true, got %q", v)
	}
	if v := normalizeDoneValue(false); v != "no" {
		t.Errorf("expected no for false, got %q", v)
	}
	if v := normalizeDoneValue("  YES  "); v != "yes" {
		t.Errorf("expected yes for trimmed string, got %q", v)
	}
	if v := normalizeDoneValue(42); v != "42" {
		t.Errorf("expected 42 for int, got %q", v)
	}
}

func TestResolveMatrixForCLI_EdgeCases(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	// 1. matrixOverride without profileOverride
	if _, err := ResolveMatrixForCLI(root, "", "", "data.csv", ""); err == nil {
		t.Errorf("expected error when matrixOverride is set without profileOverride")
	}

	// 2. matrixOverride with profileOverride
	res, err := ResolveMatrixForCLI(root, "", "", "data.csv", "profile.yaml")
	if err != nil {
		t.Fatalf("ResolveMatrixForCLI override failed: %v", err)
	}
	if res.Alias != "(override)" {
		t.Errorf("expected (override) alias, got %q", res.Alias)
	}
	if res.CSVPath != filepath.Join(root, "data.csv") {
		t.Errorf("unexpected CSVPath: %q", res.CSVPath)
	}
	if res.ProfilePath != filepath.Join(root, "profile.yaml") {
		t.Errorf("unexpected ProfilePath: %q", res.ProfilePath)
	}
}

func TestSummarizeMatrixFromPaths_Roundtrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	csvFile := filepath.Join(dir, "matrix.csv")
	profFile := filepath.Join(dir, "profile.yaml")

	csvContent := "id,name,gate1,gate2\n1,Alpha,yes,yes\n2,Beta,no,yes\n"
	_ = fileutil.WriteFile(csvFile, []byte(csvContent), paths.FilePerm600)

	profContent := "completion:\n  done_values:\n    - yes\n  gate_columns:\n    - gate1\n    - gate2\n"
	_ = fileutil.WriteFile(profFile, []byte(profContent), paths.FilePerm600)

	sum, err := SummarizeMatrixFromPaths(csvFile, profFile, false, "")
	if err != nil {
		t.Fatalf("SummarizeMatrixFromPaths failed: %v", err)
	}
	if sum.RowTotal != 2 {
		t.Errorf("expected 2 total rows, got %d", sum.RowTotal)
	}
	if sum.FullyDoneRows != 1 {
		t.Errorf("expected 1 completed row, got %d", sum.FullyDoneRows)
	}

	// Non-existent profile
	if _, err := SummarizeMatrixFromPaths(csvFile, filepath.Join(dir, "missing.yaml"), false, ""); err == nil {
		t.Errorf("expected error on missing profile file")
	}
}

func TestSentinelPipeline_StageErrors(t *testing.T) {
	t.Parallel()
	pipeCtx := &pipeline.Context{}

	// 1. Invalid payload type to toSentinelPayload
	pipeline1 := BuildSentinelPipeline(nil, &mockAnalyzer{})
	if _, err := pipeline1.Run(pipeCtx, "invalid-payload-type"); err == nil {
		t.Errorf("expected error when payload is not *SentinelPayload")
	}

	// 2. Empty VideoURL and empty Frames in extract_frames stage
	pipeline2 := BuildSentinelPipeline(nil, &mockAnalyzer{})
	emptyPayload := &SentinelPayload{
		VideoURL: "",
		Frames:   nil,
	}
	if _, err := pipeline2.Run(pipeCtx, emptyPayload); err == nil {
		t.Errorf("expected error when VideoURL and Frames are empty")
	}

	// 3. DescribeScene error
	errAnalyzer := &mockAnalyzer{
		err: errors.New("analyzer error"),
	}
	pipeline3 := BuildSentinelPipeline(nil, errAnalyzer)
	payloadWithFrames := &SentinelPayload{
		VideoURL: "http://example.com/v.mp4",
		Frames:   [][]byte{[]byte("frame1")},
	}
	if _, err := pipeline3.Run(pipeCtx, payloadWithFrames); err == nil {
		t.Errorf("expected error when DescribeScene fails")
	}

	// 4. SemanticCompare error
	compareErrAnalyzer := &semanticCompareErrAnalyzer{}
	pipeline4 := BuildSentinelPipeline(nil, compareErrAnalyzer)
	if _, err := pipeline4.Run(pipeCtx, payloadWithFrames); err == nil {
		t.Errorf("expected error when SemanticCompare fails")
	}
}

type semanticCompareErrAnalyzer struct {
	mockAnalyzer
}

func (s *semanticCompareErrAnalyzer) DescribeScene(_ context.Context, _ [][]byte) (string, error) {
	return "observed scene", nil
}

func (s *semanticCompareErrAnalyzer) SemanticCompare(_ context.Context, _, _ string) (float64, error) {
	return 0, errors.New("semantic compare failure")
}
