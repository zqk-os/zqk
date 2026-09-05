package scenario

import (
	"bytes"
	stdcontext "context"
	"encoding/json"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	_ "github.com/lanceman/zqk/pkg/specbuilder/bldr_instance_v1" // register goal builder for test
	"github.com/lanceman/zqk/pkg/specbuilder/instance_builders"
	"github.com/lanceman/zqk/pkg/storage"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/validation"
)

// persistenceBundleYAMLPath returns the path to persistence-bundle.yaml: tracked testdata first,
// then repo test-scenarios/ (often gitignored) for local runs.
func persistenceBundleYAMLPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	dir := filepath.Dir(file)
	p := filepath.Join(dir, "testdata", "persistence-bundle", "persistence-bundle.yaml")
	if _, err := fileutil.Stat(p); err == nil {
		return p
	}
	cwd, err := fileutil.Getwd()
	if err != nil {
		return ""
	}
	for d := cwd; d != emptyValue; d = filepath.Dir(d) {
		alt := filepath.Join(d, "test-scenarios", "persistence-bundle", "persistence-bundle.yaml")
		if _, err := fileutil.Stat(alt); err == nil {
			return alt
		}
		if filepath.Dir(d) == d {
			break
		}
	}
	return ""
}

func TestApplyScenarioBundle_CreatesSchedulerJobFixture(t *testing.T) {
	t.Parallel()

	// Use the standard test environment helper so directory structure and
	// config are aligned with storage expectations.
	env := setupScenarioCompleteTestEnvironment(t)
	projectRoot := env.TestRoot

	bundleYAML := `
api_version: v1
kind: scenario_bundle
metadata:
  name: scheduler-fixture-test
objects:
  fixtures:
    scheduler_jobs:
      - id_hint: SCH-BUNDLE-TEST-001
        template:
          schema_version: "` + objects.DefaultSchemaVersion + `"
          title: "Scheduler bundle test job"
          status: "active"
          job_type: "cache_prewarm"
          trigger_type: "timer"
          schedule_expression: "*/5 * * * *"
          category: "testing"
          execution_mode: "reusable"
          max_runtime_seconds: 60
          enabled: true
`

	ctx := stdcontext.Background()
	summary, err := ApplyScenarioBundle(ctx, projectRoot, bytes.NewBufferString(bundleYAML), ApplyObjectsOnly)
	if err != nil {
		t.Fatalf("ApplyScenarioBundle returned error: %v", err)
	}

	if summary.ProjectRoot != projectRoot {
		t.Errorf("summary.ProjectRoot = %q, want %q", summary.ProjectRoot, projectRoot)
	}
	if summary.BundleName != "scheduler-fixture-test" {
		t.Errorf("summary.BundleName = %q, want %q", summary.BundleName, "scheduler-fixture-test")
	}
	if len(summary.CreatedSchedulerJobIDs) != 1 {
		t.Fatalf("expected 1 created scheduler_job, got %d", len(summary.CreatedSchedulerJobIDs))
	}
	if gotID := summary.CreatedSchedulerJobIDs[0]; gotID != "SCH-BUNDLE-TEST-001" {
		t.Errorf("CreatedSchedulerJobIDs[0] = %q, want %q", gotID, "SCH-BUNDLE-TEST-001")
	}
	if mapped, ok := summary.HintToID["SCH-BUNDLE-TEST-001"]; !ok || mapped != "SCH-BUNDLE-TEST-001" {
		t.Errorf("HintToID[\"SCH-BUNDLE-TEST-001\"] = %q, ok=%v; want %q, true", mapped, ok, "SCH-BUNDLE-TEST-001")
	}
}

func TestApplyScenarioBundle_CreatesTraceabilityObjects(t *testing.T) {
	t.Parallel()

	env := setupScenarioCompleteTestEnvironment(t)
	projectRoot := env.TestRoot
	ctx := stdcontext.Background()
	secCtx := env.SecurityContext

	// Requirement spec requires goal_refs (minCount: 1). Create one goal so the bundle can reference it.
	reg := instance_builders.GetGlobalRegistry()
	goalBuilder, err := reg.GetBuilder("goal", objects.DefaultSchemaVersion)
	if err != nil {
		t.Fatalf("get goal builder: %v", err)
	}
	goalBuilder.SetID("GOAL-TRACE-001")
	goalBuilder.SetField(objects.FieldKeyTitle, "Traceability test goal")
	goalBuilder.SetStatus("active")
	goalBuilder.SetField(objects.FieldKeyAuthority, "owner")
	goalBuilder.SetField(objects.FieldKeyTarget, "1")
	goalBuilder.SetField(objects.FieldKeyOriginProject, validation.DefaultOriginProject)
	goalBuilder.SetField(objects.FieldKeyOriginSystem, validation.DefaultOriginSystem)
	goalObj, err := goalBuilder.Build()
	if err != nil {
		t.Fatalf("build goal: %v", err)
	}
	provider, ok := env.Storage.(storage.ObjectStorageProvider)
	if !ok {
		t.Fatalf("env.Storage is not ObjectStorageProvider")
	}
	if err := provider.Create(ctx, secCtx, goalObj); err != nil {
		t.Fatalf("create goal: %v", err)
	}

	bundleYAML := `
api_version: v1
kind: scenario_bundle
metadata:
  name: traceability-test
objects:
  requirements:
    - id_hint: REQ-TRACE-001
      title: Traceability test requirement
      goal_refs: [GOAL-TRACE-001]
      criteria_refs: [CRIT-TRACE-001]
  criteria:
    - id_hint: CRIT-TRACE-001
      title: Traceability test criteria
      description: "Verifies requirement → criteria link"
      # requirement_ref omitted to avoid Update() in test (storage Save can block in test env); requirement still has criteria_refs
  test_cases:
    - id_hint: TEST-TRACE-001
      title: Traceability test case
      requirement_refs: [REQ-TRACE-001]
      criteria_refs: [CRIT-TRACE-001]
      test_functions: ["pkg/scenario.TestApplyScenarioBundle_CreatesTraceabilityObjects"]
  backlog_items:
    - id_hint: BLI-TRACE-001
      title: Traceability backlog item
      requirement_refs: [REQ-TRACE-001]
      criteria_refs: [CRIT-TRACE-001]
      test_case_refs: [TEST-TRACE-001]
`

	// Use same storage as goal creation so criteria created by the bundle are visible when validating requirement refs.
	summary, err := ApplyScenarioBundle(ctx, projectRoot, bytes.NewBufferString(bundleYAML), ApplyObjectsOnly, &ApplyOptions{Storage: provider})
	if err != nil {
		t.Fatalf("ApplyScenarioBundle returned error: %v", err)
	}

	if len(summary.CreatedRequirementIDs) != 1 || summary.CreatedRequirementIDs[0] != "REQ-TRACE-001" {
		t.Errorf("CreatedRequirementIDs = %v, want [REQ-TRACE-001]", summary.CreatedRequirementIDs)
	}
	if len(summary.CreatedCriteriaIDs) != 1 || summary.CreatedCriteriaIDs[0] != "CRIT-TRACE-001" {
		t.Errorf("CreatedCriteriaIDs = %v, want [CRIT-TRACE-001]", summary.CreatedCriteriaIDs)
	}
	if len(summary.CreatedTestCaseIDs) != 1 || summary.CreatedTestCaseIDs[0] != "TEST-TRACE-001" {
		t.Errorf("CreatedTestCaseIDs = %v, want [TEST-TRACE-001]", summary.CreatedTestCaseIDs)
	}
	if len(summary.CreatedBacklogItemIDs) != 1 || summary.CreatedBacklogItemIDs[0] != "BLI-TRACE-001" {
		t.Errorf("CreatedBacklogItemIDs = %v, want [BLI-TRACE-001]", summary.CreatedBacklogItemIDs)
	}
	for _, hint := range []string{"REQ-TRACE-001", "CRIT-TRACE-001", "TEST-TRACE-001", "BLI-TRACE-001"} {
		if id, ok := summary.HintToID[hint]; !ok || id != hint {
			t.Errorf("HintToID[%q] = %q, ok=%v; want %q, true", hint, id, ok, hint)
		}
	}

	// Verify objects exist in storage (Read by ID; ID is globally unique)
	for _, id := range []string{"REQ-TRACE-001", "CRIT-TRACE-001", "TEST-TRACE-001", "BLI-TRACE-001"} {
		obj, err := provider.Read(ctx, secCtx, id)
		if err != nil {
			t.Errorf("Read(%q): %v", id, err)
			continue
		}
		if obj == nil {
			t.Errorf("Read(%q): object nil", id)
		}
	}
}

// processHygieneConfigTraceabilityBundlePath returns tracked testdata for the process hygiene config bundle.
func processHygieneConfigTraceabilityBundlePath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	dir := filepath.Dir(file)
	return filepath.Join(dir, "testdata", "process-hygiene-config-traceability-bundle", "process-hygiene-config-traceability-bundle.yaml")
}

// TestApplyScenarioBundle_ProcessHygieneConfigTraceabilityBundleLoad decodes the hygiene traceability bundle and checks shape.
func TestApplyScenarioBundle_ProcessHygieneConfigTraceabilityBundleLoad(t *testing.T) {
	t.Parallel()

	path := processHygieneConfigTraceabilityBundlePath(t)
	data, err := fileutil.ReadFile(path)
	if err != nil {
		t.Fatalf("read bundle %q: %v", path, err)
	}
	bundle, err := LoadBundle(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("LoadBundle: %v", err)
	}
	if bundle.Metadata.Name != "process-hygiene-config-traceability-bundle" {
		t.Errorf("metadata.name = %q", bundle.Metadata.Name)
	}
	if len(bundle.Objects.Goals) != 1 {
		t.Errorf("goals: want 1, got %d", len(bundle.Objects.Goals))
	}
	if len(bundle.Objects.Requirements) != 1 {
		t.Errorf("requirements: want 1, got %d", len(bundle.Objects.Requirements))
	}
	if len(bundle.Objects.Requirements[0].GoalRefs) == 0 {
		t.Error("requirement must have goal_refs")
	}
	if len(bundle.Objects.Criteria) != 5 {
		t.Errorf("criteria: want 5, got %d", len(bundle.Objects.Criteria))
	}
	if len(bundle.Objects.TestCases) != 1 {
		t.Errorf("test_cases: want 1, got %d", len(bundle.Objects.TestCases))
	}
	if len(bundle.Objects.BacklogItems) != 2 {
		t.Errorf("backlog_items: want 2, got %d", len(bundle.Objects.BacklogItems))
	}
	if len(bundle.Objects.DocEntries) != 2 {
		t.Errorf("doc_entries: want 2, got %d", len(bundle.Objects.DocEntries))
	}
}

func TestApplyScenarioBundle_CapLoopHonestyTraceabilityBundleLoad(t *testing.T) {
	t.Parallel()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	path := filepath.Join(filepath.Dir(file), "testdata", "cap-loop-honesty-traceability-bundle", "cap-loop-honesty-traceability-bundle.yaml")
	data, err := fileutil.ReadFile(path)
	if err != nil {
		t.Fatalf("read bundle %q: %v", path, err)
	}
	bundle, err := LoadBundle(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("LoadBundle: %v", err)
	}
	if bundle.Metadata.Name != "cap-loop-honesty-traceability-bundle" {
		t.Fatalf("name=%q", bundle.Metadata.Name)
	}
	if len(bundle.Objects.Goals) != 1 {
		t.Errorf("goals: want 1, got %d", len(bundle.Objects.Goals))
	}
	if len(bundle.Objects.Requirements) != 4 {
		t.Errorf("requirements: want 4, got %d", len(bundle.Objects.Requirements))
	}
	if len(bundle.Objects.Criteria) != 12 {
		t.Errorf("criteria: want 12, got %d", len(bundle.Objects.Criteria))
	}
	if len(bundle.Objects.BacklogItems) != 4 {
		t.Errorf("backlog_items: want 4, got %d", len(bundle.Objects.BacklogItems))
	}
	if len(bundle.Objects.TestCases) != 3 {
		t.Errorf("test_cases: want 3, got %d", len(bundle.Objects.TestCases))
	}
	if len(bundle.Objects.DocEntries) != 3 {
		t.Errorf("doc_entries: want 3, got %d", len(bundle.Objects.DocEntries))
	}
}

// TestApplyScenarioBundle_PersistenceBundleLoad wires the persistence-bundle into the test suite:
// load the bundle file and assert its shape (goals, goal_refs on requirement, criteria).
func TestApplyScenarioBundle_PersistenceBundleLoad(t *testing.T) {
	t.Parallel()

	bundlePath := persistenceBundleYAMLPath(t)
	if bundlePath == emptyValue {
		t.Skip("persistence-bundle.yaml not found (testdata or test-scenarios/)")
	}

	f, err := fileutil.Open(bundlePath)
	if err != nil {
		t.Fatalf("open bundle: %v", err)
	}
	defer f.Close()

	bundle, err := LoadBundle(f)
	if err != nil {
		t.Fatalf("LoadBundle: %v", err)
	}
	if len(bundle.Objects.Goals) < 1 {
		t.Errorf("persistence-bundle should define at least 1 goal; got %d", len(bundle.Objects.Goals))
	}
	if len(bundle.Objects.Requirements) < 1 {
		t.Errorf("persistence-bundle should define at least 1 requirement; got %d", len(bundle.Objects.Requirements))
	}
	if len(bundle.Objects.Requirements[0].GoalRefs) == 0 {
		t.Error("persistence-bundle requirement must have goal_refs (spec minCount: 1)")
	}
	if len(bundle.Objects.Criteria) < 2 {
		t.Errorf("persistence-bundle should define at least 2 criteria; got %d", len(bundle.Objects.Criteria))
	}
	kinds := bundleTraceabilityKinds(bundle)
	var hasGoal bool
	for _, k := range kinds {
		if k == "goal" {
			hasGoal = true
			break
		}
	}
	if !hasGoal {
		t.Errorf("bundleTraceabilityKinds should include goal when bundle has goals; got %v", kinds)
	}
}

// TestApplyScenarioBundle_EmitsScenarioSummary asserts that after a successful apply,
// .zqk/scenarios/<bundle-name>/scenario-summary.json is written with expected shape.
func TestApplyScenarioBundle_EmitsScenarioSummary(t *testing.T) {
	t.Parallel()

	env := setupScenarioCompleteTestEnvironment(t)
	ctx := stdcontext.Background()
	projectRoot := env.TestRoot
	provider, ok := env.Storage.(storage.ObjectStorageProvider)
	if !ok {
		t.Fatalf("env.Storage is not ObjectStorageProvider")
	}

	// Use minimal bundle (goal + requirement + criteria without requirement_ref to avoid Update path).
	reg := instance_builders.GetGlobalRegistry()
	goalBuilder, _ := reg.GetBuilder("goal", objects.DefaultSchemaVersion)
	goalBuilder.SetID("GOAL-SUM-001")
	goalBuilder.SetField(objects.FieldKeyTitle, "Summary test goal")
	goalBuilder.SetStatus("active")
	goalBuilder.SetField(objects.FieldKeyAuthority, "owner")
	goalBuilder.SetField(objects.FieldKeyTarget, "1")
	goalBuilder.SetField(objects.FieldKeyOriginProject, validation.DefaultOriginProject)
	goalBuilder.SetField(objects.FieldKeyOriginSystem, validation.DefaultOriginSystem)
	goalObj, _ := goalBuilder.Build()
	_ = provider.Create(ctx, env.SecurityContext, goalObj)

	bundleYAML := `
api_version: v1
kind: scenario_bundle
metadata:
  name: summary-test
objects:
  requirements:
    - id_hint: REQ-SUM-001
      title: Summary test requirement
      goal_refs: [GOAL-SUM-001]
      criteria_refs: [CRIT-SUM-001]
  criteria:
    - id_hint: CRIT-SUM-001
      title: Summary test criteria
`
	summary, err := ApplyScenarioBundle(ctx, projectRoot, bytes.NewBufferString(bundleYAML), ApplyObjectsOnly, &ApplyOptions{Storage: provider})
	if err != nil {
		t.Fatalf("ApplyScenarioBundle: %v", err)
	}

	summaryPath := filepath.Join(scenarioSummaryDir(projectRoot, summary.BundleName), "scenario-summary.json")
	data, err := fileutil.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read scenario-summary.json: %v", err)
	}
	var written BundleSummary
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatalf("unmarshal scenario-summary.json: %v", err)
	}
	if written.BundleName != summary.BundleName {
		t.Errorf("written summary bundle_name = %q, want %q", written.BundleName, summary.BundleName)
	}
	if len(written.HintToID) == 0 {
		t.Error("written summary hint_to_id is empty")
	}
}

// convergenceLifecycleBundlePath returns testdata convergence-lifecycle-bundle.yaml.
func convergenceLifecycleBundlePath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	dir := filepath.Dir(file)
	return filepath.Join(dir, "testdata", "convergence-lifecycle-bundle", "convergence-lifecycle-bundle.yaml")
}

// TestApplyScenarioBundle_ConvergenceLifecycleBundle applies the full convergence scaffold (goal, criteria,
// requirement, doc_entry, convergence_session, backlog) in one apply.
func TestApplyScenarioBundle_ConvergenceLifecycleBundle(t *testing.T) {
	t.Parallel()

	env := setupScenarioCompleteTestEnvironment(t)
	projectRoot := env.TestRoot
	ctx := stdcontext.Background()
	secCtx := env.SecurityContext

	path := convergenceLifecycleBundlePath(t)
	data, err := fileutil.ReadFile(path)
	if err != nil {
		t.Fatalf("read bundle %q: %v", path, err)
	}
	provider, ok := env.Storage.(storage.ObjectStorageProvider)
	if !ok {
		t.Fatalf("env.Storage is not ObjectStorageProvider")
	}

	summary, err := ApplyScenarioBundle(ctx, projectRoot, bytes.NewBuffer(data), ApplyObjectsOnly, &ApplyOptions{Storage: provider})
	if err != nil {
		t.Fatalf("ApplyScenarioBundle: %v", err)
	}

	if len(summary.CreatedGoalIDs) != 1 || summary.CreatedGoalIDs[0] != "GOAL-CLF-001" {
		t.Errorf("CreatedGoalIDs = %v, want [GOAL-CLF-001]", summary.CreatedGoalIDs)
	}
	if len(summary.CreatedCriteriaIDs) != 4 {
		t.Errorf("CreatedCriteriaIDs len = %d, want 4", len(summary.CreatedCriteriaIDs))
	}
	if len(summary.CreatedRequirementIDs) != 1 || summary.CreatedRequirementIDs[0] != "REQ-CLF-001" {
		t.Errorf("CreatedRequirementIDs = %v, want [REQ-CLF-001]", summary.CreatedRequirementIDs)
	}
	if len(summary.CreatedDocEntryIDs) != 1 || summary.CreatedDocEntryIDs[0] != "DOC-CLF-001" {
		t.Errorf("CreatedDocEntryIDs = %v, want [DOC-CLF-001]", summary.CreatedDocEntryIDs)
	}
	wantCVS := "REDACTED"
	if len(summary.CreatedConvergenceSessionIDs) != 1 || summary.CreatedConvergenceSessionIDs[0] != wantCVS {
		t.Errorf("CreatedConvergenceSessionIDs = %v, want [%s]", summary.CreatedConvergenceSessionIDs, wantCVS)
	}
	if len(summary.CreatedBacklogItemIDs) != 1 || summary.CreatedBacklogItemIDs[0] != "BLI-CLF-001" {
		t.Errorf("CreatedBacklogItemIDs = %v, want [BLI-CLF-001]", summary.CreatedBacklogItemIDs)
	}

	for _, tc := range []struct {
		id       string
		wantKind string
	}{
		{"DOC-CLF-001", "doc_entry"},
		{"REDACTED", "convergence_session"},
	} {
		obj, err := provider.Read(ctx, secCtx, tc.id)
		if err != nil {
			t.Fatalf("Read %q: %v", tc.id, err)
		}
		got, _ := obj[objects.FieldKeyKind].(string)
		if got != tc.wantKind {
			t.Errorf("object %q kind = %q, want %q", tc.id, got, tc.wantKind)
		}
	}

	// Parent owns the edge: requirement.criteria_refs (not criteria.requirement_refs).
	req, err := provider.Read(ctx, secCtx, "REQ-CLF-001")
	if err != nil {
		t.Fatalf("Read REQ-CLF-001: %v", err)
	}
	refs, _ := req[objects.FieldKeyCriteriaRefs].([]any)
	wantRefs := []string{"CRIT-CLF-001", "CRIT-CLF-002", "CRIT-CLF-003", "CRIT-CLF-004"}
	if len(refs) != len(wantRefs) {
		t.Errorf("REQ-CLF-001 criteria_refs = %v, want %v", refs, wantRefs)
	} else {
		for i, want := range wantRefs {
			s, ok := refs[i].(string)
			if !ok || s != want {
				t.Errorf("REQ-CLF-001 criteria_refs[%d] = %v, want %s", i, refs[i], want)
			}
		}
	}
	crit, err := provider.Read(ctx, secCtx, "CRIT-CLF-001")
	if err != nil {
		t.Fatalf("Read CRIT-CLF-001: %v", err)
	}
	if _, ok := crit[objects.FieldKeyRequirementRefs]; ok {
		t.Errorf("CRIT-CLF-001 must not store requirement_refs; got %v", crit[objects.FieldKeyRequirementRefs])
	}
}

func TestApplyScenarioBundle_TraceabilityLinkageQADisparityRemediation(t *testing.T) {
	t.Parallel()
	env := setupScenarioCompleteTestEnvironment(t)
	bundleYAML := `
api_version: v1
kind: scenario_bundle
metadata:
  name: qa-disparity-trace-remediation-test
  description: Verifies TRACE test linkage and QA disparity interrupt remediation.
objects:
  goals:
    - id_hint: GOAL-QAD-001
      title: QA Disparity Remediation Goal
      status: active
      authority: owner
      target: "1"
  requirements:
    - id_hint: REQ-QAD-001
      title: QA Disparity Remediation Requirement
      status: proposed
      goal_refs: [GOAL-QAD-001]
      criteria_refs: [CRIT-QAD-001]
  criteria:
    - id_hint: CRIT-QAD-001
      title: QA Disparity Remediation Criteria
      requirement_ref: REQ-QAD-001
      category: acceptance
      validation_method: code_review
  backlog_items:
    - id_hint: BLI-QAD-001
      title: QA Disparity Remediation Backlog Item
      status: exploring
      requirement_refs: [REQ-QAD-001]
      criteria_refs: [CRIT-QAD-001]
`
	projectRoot := env.TestRoot
	provider, ok := env.Storage.(storage.ObjectStorageProvider)
	if !ok || provider == nil {
		t.Fatalf("env.Storage is not ObjectStorageProvider")
	}
	summary, err := ApplyScenarioBundle(stdcontext.Background(), projectRoot, bytes.NewBufferString(bundleYAML), ApplyObjectsOnly, &ApplyOptions{Storage: provider})
	if err != nil {
		t.Fatalf("ApplyScenarioBundle failed: %v", err)
	}
	if summary == nil || len(summary.CreatedBacklogItemIDs) == 0 {
		t.Fatalf("ApplyScenarioBundle returned empty summary")
	}
}
