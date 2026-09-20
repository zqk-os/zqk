package scheduler

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestCEFMeasureFromRows_envelopeBelowFloor(t *testing.T) {
	t.Parallel()
	rows := []map[string]string{
		{
			"seat_role": "envelope", "envelope_min": "3", "package_complete": "yes",
			"freeze_sha": "abc", "assessed_at": "2026-08-31", "run_id": "ENV",
		},
	}
	res, err := cefMeasureFromRows("cef_diamond_scorecard", "/tmp/x.csv", rows, 4)
	if err != nil {
		t.Fatal(err)
	}
	if res.DeltaAssessment != "trending_away" {
		t.Fatalf("delta: %q", res.DeltaAssessment)
	}
	if res.ReadyForSessionCompletion {
		t.Fatal("expected not ready")
	}
	if res.AfterStateSnapshot["evaluation_surface_id"] != EvaluationSurfaceCEFDiamondScorecard {
		t.Fatalf("surface: %#v", res.AfterStateSnapshot["evaluation_surface_id"])
	}
}

func TestCEFMeasureFromRows_BLI_CEF_R28_REMEASURE_001_EnvelopeMeetsFloor(t *testing.T) {
	t.Parallel()
	rows := []map[string]string{
		{
			"seat_role": "envelope", "envelope_min": "4", "package_complete": "yes",
			"freeze_sha": "c543a2813d208175222af6d44b0045fd42bc9d86", "assessed_at": "2026-09-07", "run_id": "ENV",
		},
	}
	res, err := cefMeasureFromRows("cef_diamond_scorecard", "/tmp/x.csv", rows, 4)
	if err != nil {
		t.Fatal(err)
	}
	if res.DeltaAssessment != "trending_toward" {
		t.Fatalf("delta: %q, want trending_toward", res.DeltaAssessment)
	}
	if !res.ReadyForSessionCompletion {
		t.Fatal("expected ready for session completion when envelope_min>=4")
	}
	if len(res.SessionCompletionBlockedReasons) != 0 {
		t.Fatalf("unexpected blocked reasons: %v", res.SessionCompletionBlockedReasons)
	}
	if res.AfterStateSnapshot["last_complete_envelope_min"] != 4 {
		t.Fatalf("envelope: %#v, want 4", res.AfterStateSnapshot["last_complete_envelope_min"])
	}
}

func TestBuildCEFDiamondMeasureResult_fixture(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	qualityDir := filepath.Join(root, "docs", "quality")
	if err := fileutil.MkdirAll(qualityDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	reg := `default: cef_diamond_scorecard
matrices:
  cef_diamond_scorecard:
    csv: docs/quality/CEF_DIAMOND_SCORECARD_MATRIX.csv
    profile: docs/quality/cef_diamond_scorecard_matrix_profile.yaml
    session_ref_column: cvs_id
`
	csv := `run_id,seat_role,agent_run_id,seat_agent_id,freeze_sha,assessed_at,RDB,MNT,TST,REL,OBS,RCV,SEC,ROB,envelope_min,package_complete,output_home,scorecard_path,cvs_id,bli_ref,notes
ENV,envelope,min,,sha1,2026-08-31,3,3,3,3,3,3,3,3,3,yes,,,CVS-TEST,,
`
	prof := `schema_version: 1
fieldnames: [run_id]
completion:
  gate_columns: [package_complete]
  done_values: [yes, na]
`
	if err := fileutil.WriteFile(filepath.Join(qualityDir, "matrix_registry.yaml"), []byte(reg), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(qualityDir, "CEF_DIAMOND_SCORECARD_MATRIX.csv"), []byte(csv), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(qualityDir, "cef_diamond_scorecard_matrix_profile.yaml"), []byte(prof), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	res, err := BuildCEFDiamondMeasureResult(root, "CVS-TEST", map[string]any{"min_axis_grade": 4})
	if err != nil {
		t.Fatal(err)
	}
	if res.AfterStateSnapshot["last_complete_envelope_min"] != 3 {
		t.Fatalf("envelope: %#v", res.AfterStateSnapshot["last_complete_envelope_min"])
	}
	if res.EvaluationSurfaceID != EvaluationSurfaceCEFDiamondScorecard {
		t.Fatalf("surface id %q", res.EvaluationSurfaceID)
	}
}
