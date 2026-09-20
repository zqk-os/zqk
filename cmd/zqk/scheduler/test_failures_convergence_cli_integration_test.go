package scheduler

// BLI-177483 inventory: object.SetupTestEnvironment → testkit.RunStandardTeardown (TempProjectTeardown) in cmd/zqk/object/test_helpers.go.

import (
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/cmd/zqk/object"
	schedpkg "github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/zqk-os/zqk/pkg/zqktime"

	"github.com/zqk-os/zqk/pkg/convergerollup"
	"github.com/zqk-os/zqk/pkg/nildecode"
	"github.com/zqk-os/zqk/pkg/objects"
)

// TestCLI_TestFailuresConvergence_JSON_Integration runs the real zqk binary against an isolated
// project (ZQK_TEST_ROOT): writes health.jsonl, invokes `scheduler convergence measure`,
// creates a convergence_session via `object create`, then re-runs with `--session-id` and asserts
// suggested_convergence_session_fields (complements pkg/scenario storage-backed E2E tests).
func TestCLI_TestFailuresConvergence_JSON_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skip CLI integration in -short mode")
	}

	te := object.SetupTestEnvironment(t)
	root := te.GetTestRoot()

	healthPath := schedpkg.TestBundlesHealthFilePath(root)
	if err := fileutil.EnsureDir(filepath.Dir(healthPath)); err != nil {
		t.Fatalf("mkdir health dir: %v", err)
	}
	ts := zqktime.NowRFC3339UTC()
	line := map[string]any{
		schedpkg.KeyTimestamp:                ts,
		schedpkg.KeyBundleCommandFingerprint: "cli-int-fp",
		schedpkg.KeyTestOutcome:              "pass",
	}
	b, err := json.Marshal(line)
	if err != nil {
		t.Fatalf("marshal health line: %v", err)
	}
	if err := fileutil.WriteSecureFile(healthPath, append(b, '\n')); err != nil {
		t.Fatalf("write health.jsonl: %v", err)
	}

	outNoSession := runZQKCLI(t, te, "scheduler", "convergence", "measure", "--format", "json", "--skip-rollup-gates")
	topNo := parseJSONObjectFromCLI(t, outNoSession)
	if topNo[objects.FieldKeyDeltaAssessment] != "neutral" {
		t.Fatalf("delta_assessment: got %v", topNo[objects.FieldKeyDeltaAssessment])
	}
	if topNo[objects.FieldKeyPrimaryMeasurementOutcome] != string(convergerollup.MeasurementYieldsConvergence) {
		t.Fatalf("primary_measurement_outcome: got %v want %s", topNo[objects.FieldKeyPrimaryMeasurementOutcome], convergerollup.MeasurementYieldsConvergence)
	}
	if topNo["measurement_outcome_schema_version"] != "1" {
		t.Fatalf("measurement_outcome_schema_version: got %v", topNo["measurement_outcome_schema_version"])
	}
	if _, ok := topNo["suggested_convergence_session_fields"]; ok {
		t.Fatalf("expected no suggested_convergence_session_fields without --session-id: keys present")
	}
	rscNoRaw, ok := topNo["rollup_status_core"]
	if !ok {
		t.Fatalf("expected rollup_status_core.rollup_status without --session-id: %#v", topNo["rollup_status_core"])
	}
	rscNoRaw, ok = nildecode.DecodeNonNilPayload[any](rscNoRaw)
	if !ok {
		t.Fatalf("expected rollup_status_core.rollup_status without --session-id: %#v", topNo["rollup_status_core"])
	}
	rscNo, ok := rscNoRaw.(map[string]any)
	if !ok || rscNo["rollup_status"] == nil {
		t.Fatalf("expected rollup_status_core.rollup_status without --session-id: %#v", topNo["rollup_status_core"])
	}
	if skipped, _ := rscNo["rollup_gates_skipped"].(bool); !skipped {
		t.Fatalf("expected rollup_gates_skipped true with --skip-rollup-gates, got %#v", rscNo["rollup_gates_skipped"])
	}

	cvsYAML := filepath.Join(t.TempDir(), "cvs-e2e-cli.yaml")
	const cvsID = "CVS-E2E-CLI-001"
	cvsContent := `id: ` + cvsID + `
kind: convergence_session
title: CLI integration test session
status: draft
current_phase: c1_scope
outcome_character: pending
delta_assessment: unknown
schema_version: "` + objects.DefaultSchemaVersion + `"
namespace_id: zqk:kernel
hypothesis: |
  integration test.
desired_end_state: |
  green.
next_action: |
  measure.
`
	if err := fileutil.WriteSecureFile(cvsYAML, []byte(cvsContent)); err != nil {
		t.Fatalf("write cvs yaml: %v", err)
	}

	runZQKCLI(t, te, "object", "create", "convergence_session", "--file", cvsYAML)

	outSession := runZQKCLI(t, te, "scheduler", "convergence", "measure", "--format", "json", "--session-id", cvsID, "--skip-rollup-gates")
	top := parseJSONObjectFromCLI(t, outSession)
	if top["convergence_session_id"] != cvsID {
		t.Fatalf("convergence_session_id: got %v", top["convergence_session_id"])
	}
	sugRaw, ok := top["suggested_convergence_session_fields"]
	if !ok {
		t.Fatalf("suggested_convergence_session_fields: %T %v", top["suggested_convergence_session_fields"], top["suggested_convergence_session_fields"])
	}
	sugRaw, ok = nildecode.DecodeNonNilPayload[any](sugRaw)
	if !ok {
		t.Fatalf("suggested_convergence_session_fields: %T %v", top["suggested_convergence_session_fields"], top["suggested_convergence_session_fields"])
	}
	sug, ok := sugRaw.(map[string]any)
	if !ok {
		t.Fatalf("suggested_convergence_session_fields: %T %v", top["suggested_convergence_session_fields"], top["suggested_convergence_session_fields"])
	}
	if sug[objects.FieldKeyDeltaAssessment] != "neutral" {
		t.Fatalf("suggested delta_assessment: %v", sug[objects.FieldKeyDeltaAssessment])
	}
	ouRaw, ok := sug["object_update_body"]
	if !ok {
		t.Fatalf("object_update_body: %#v", sug["object_update_body"])
	}
	ouRaw, ok = nildecode.DecodeNonNilPayload[any](ouRaw)
	if !ok {
		t.Fatalf("object_update_body: %#v", sug["object_update_body"])
	}
	ou, ok := ouRaw.(map[string]any)
	if !ok {
		t.Fatalf("object_update_body: %#v", sug["object_update_body"])
	}
	for _, k := range []string{"delta_assessment", "after_state_snapshot", "next_action", "current_phase", "activity_log"} {
		if _, ok := ou[k]; !ok {
			t.Fatalf("object_update_body missing %q: %#v", k, ou)
		}
	}
	prRaw, ok := sug["phase_router"]
	if !ok {
		t.Fatalf("phase_router: %#v", sug["phase_router"])
	}
	prRaw, ok = nildecode.DecodeNonNilPayload[any](prRaw)
	if !ok {
		t.Fatalf("phase_router: %#v", sug["phase_router"])
	}
	pr, ok := prRaw.(map[string]any)
	if !ok || pr["measurement_implied_phase"] == nil {
		t.Fatalf("phase_router: %#v", sug["phase_router"])
	}
	rscRaw, ok := top["rollup_status_core"]
	if !ok {
		t.Fatalf("expected rollup_status_core with --session-id: %#v", top["rollup_status_core"])
	}
	rscRaw, ok = nildecode.DecodeNonNilPayload[any](rscRaw)
	if !ok {
		t.Fatalf("expected rollup_status_core with --session-id: %#v", top["rollup_status_core"])
	}
	rsc, ok := rscRaw.(map[string]any)
	if !ok || rsc["rollup_status"] == nil {
		t.Fatalf("expected rollup_status_core with --session-id: %#v", top["rollup_status_core"])
	}
	if skipped, _ := rsc["rollup_gates_skipped"].(bool); !skipped {
		t.Fatalf("expected rollup_gates_skipped true with --skip-rollup-gates, got %#v", rsc["rollup_gates_skipped"])
	}
}

// TestCLI_TestFailuresConvergence_JSON_Integration_FailingHealthYieldsDivergence asserts a real CLI run
// classifies latest test-bundle failure as divergence (not halt) when the harness completed — complementing
// TestComputePrimaryMeasurementOutcome_* in pkg/convergerollup and the passing-health case above.
func TestCLI_TestFailuresConvergence_JSON_Integration_FailingHealthYieldsDivergence(t *testing.T) {
	if testing.Short() {
		t.Skip("skip CLI integration in -short mode")
	}

	te := object.SetupTestEnvironment(t)
	root := te.GetTestRoot()

	healthPath := schedpkg.TestBundlesHealthFilePath(root)
	if err := fileutil.EnsureDir(filepath.Dir(healthPath)); err != nil {
		t.Fatalf("mkdir health dir: %v", err)
	}
	ts := zqktime.NowRFC3339UTC()
	line := map[string]any{
		schedpkg.KeyTimestamp:                ts,
		schedpkg.KeyBundleCommandFingerprint: "cli-int-fp-fail",
		schedpkg.KeyTestOutcome:              "test_fail",
		schedpkg.KeySuggestedRerunCommands:   []any{"go test ./pkg/foo -run TestX -timeout 60s"},
	}
	b, err := json.Marshal(line)
	if err != nil {
		t.Fatalf("marshal health line: %v", err)
	}
	if err := fileutil.WriteSecureFile(healthPath, append(b, '\n')); err != nil {
		t.Fatalf("write health.jsonl: %v", err)
	}

	out := runZQKCLI(t, te, "scheduler", "convergence", "measure", "--format", "json", "--skip-rollup-gates")
	top := parseJSONObjectFromCLI(t, out)
	if top[objects.FieldKeyDeltaAssessment] != "trending_away" {
		t.Fatalf("delta_assessment: got %v want trending_away", top[objects.FieldKeyDeltaAssessment])
	}
	if top[objects.FieldKeyPrimaryMeasurementOutcome] != string(convergerollup.MeasurementYieldsDivergence) {
		t.Fatalf("primary_measurement_outcome: got %v want %s", top[objects.FieldKeyPrimaryMeasurementOutcome], convergerollup.MeasurementYieldsDivergence)
	}
	if top["measurement_outcome_schema_version"] != "1" {
		t.Fatalf("measurement_outcome_schema_version: got %v", top["measurement_outcome_schema_version"])
	}
}

func runZQKCLI(t *testing.T, te *object.TestEnvironment, args ...string) []byte {
	t.Helper()
	cmd := te.CreateCLICommand(args...)
	zqkenv.WireExecForIsolatedProject(cmd, te.GetTestRoot())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("zqk %v: %v\n%s", args, err, out)
	}
	return out
}

// parseJSONObjectFromCLI extracts the first JSON object from stdout (handles rare leading noise).
func parseJSONObjectFromCLI(t *testing.T, data []byte) map[string]any {
	t.Helper()
	s := strings.TrimSpace(string(data))
	start := strings.Index(s, "{")
	if start < 0 {
		t.Fatalf("no JSON object in output:\n%s", data)
	}
	// Decode one value; if trailing noise, unmarshal only the first object by scanning braces — simple path: full string from first {
	decoder := json.NewDecoder(strings.NewReader(s[start:]))
	decoder.UseNumber()
	var m map[string]any
	if err := decoder.Decode(&m); err != nil {
		t.Fatalf("json decode: %v\noutput:\n%s", err, data)
	}
	return m
}
