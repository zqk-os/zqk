package scheduler

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestCRIT9038_PolicyEngine_EvaluateExecutesWhenNoState(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	reg := NewJobStateRegistry(tmpDir)
	pe := NewPolicyEngine(reg, tmpDir, nil)

	decision, err := pe.(*PolicyEngine).Evaluate("SCH-12", &ScheduledJob{ID: "SCH-12"})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if decision.Action != decisionExecute {
		t.Fatalf("expected action=%q, got %q", decisionExecute, decision.Action)
	}
}

func TestPolicyEngine_ReconcilesStaleInProgressFromOtherPID(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.SchedulerDir, paths.StateDir)
	if err := fileutil.EnsureDir(stateDir); err != nil {
		t.Fatal(err)
	}
	st := JobExecutionState{
		JobID:       "SCH-stale-pid",
		ExecutionID: "exec-old",
		State:       jobExecutionStateInProgress,
		ProcessID:   1,
		StartedAt:   time.Now().UTC().Add(-10 * time.Minute),
	}
	b, err := yaml.Marshal(&st)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(stateDir, "SCH-stale-pid-exec-old.yaml")
	if err := fileutil.WriteSecureFile(path, b); err != nil {
		t.Fatal(err)
	}

	reg := NewJobStateRegistry(tmpDir)
	pe := NewPolicyEngine(reg, tmpDir, nil)
	decision, err := pe.(*PolicyEngine).Evaluate("SCH-stale-pid", &ScheduledJob{ID: "SCH-stale-pid", MaxRuntimeSeconds: 300})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if decision.Action != decisionExecute {
		t.Fatalf("expected stale in_progress to reconcile to execute; got action=%q reason=%q", decision.Action, decision.Reason)
	}
}

func TestCRIT9038_PolicyEngine_EvaluateSkipsWhenInProgress(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	reg := NewJobStateRegistry(tmpDir)

	jobID := "SCH-10"
	execID := "exec-10"
	if err := reg.RegisterExecution(jobID, execID, 111); err != nil {
		t.Fatalf("RegisterExecution failed: %v", err)
	}

	pe := NewPolicyEngine(reg, tmpDir, nil)
	decision, err := pe.(*PolicyEngine).Evaluate(jobID, &ScheduledJob{ID: jobID})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if decision.Action != decisionSkip {
		t.Fatalf("expected action=%q, got %q", decisionSkip, decision.Action)
	}
	if decision.Reason == emptyValue {
		t.Fatalf("expected non-empty Reason")
	}
}

func TestCRIT9038_PolicyEngine_EvaluateSkipsWhenDeferred(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	reg := NewJobStateRegistry(tmpDir)

	jobID := "SCH-11"
	execID := "exec-11"
	if err := reg.RegisterExecution(jobID, execID, 222); err != nil {
		t.Fatalf("RegisterExecution failed: %v", err)
	}

	deferUntil := time.Now().UTC().Add(10 * time.Minute)
	if err := reg.DeferExecution(jobID, "dependency_not_met", &deferUntil); err != nil {
		t.Fatalf("DeferExecution failed: %v", err)
	}

	pe := NewPolicyEngine(reg, tmpDir, nil)
	decision, err := pe.(*PolicyEngine).Evaluate(jobID, &ScheduledJob{ID: jobID})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if decision.Action != decisionSkip {
		t.Fatalf("expected action=%q, got %q", decisionSkip, decision.Action)
	}
}

func TestCRIT9041_PolicyEngine_SaveAndLoad_PersistsToDisk(t *testing.T) {
	t.Parallel()

	tmpRoot := t.TempDir()
	reg := NewJobStateRegistry(tmpRoot)
	pe := NewPolicyEngine(reg, tmpRoot, nil)

	jobID := "SCH-9041-1"
	policy := &ExecutionPolicy{
		JobID:         jobID,
		DefaultAction: decisionExecute,
		Rules: []PolicyRule{
			{
				Type:   "dependency_check",
				Action: decisionDefer,
				Reason: "dependency_not_met",
				Condition: map[string]any{
					objects.FieldKeyDependencies: []string{"SCH-9041-2"},
				},
			},
		},
	}

	if err := pe.(*PolicyEngine).SavePolicy(policy); err != nil {
		t.Fatalf("SavePolicy failed: %v", err)
	}

	path := pe.(*PolicyEngine).policyFilePath(jobID)
	if path == emptyValue {
		t.Fatalf("expected non-empty policy file path")
	}
	if _, err := fileutil.Stat(path); err != nil {
		t.Fatalf("expected policy file to exist at %q: %v", path, err)
	}

	loaded, err := pe.(*PolicyEngine).LoadPolicy(jobID)
	if err != nil {
		t.Fatalf("LoadPolicy failed: %v", err)
	}
	if loaded == nil {
		t.Fatalf("expected loaded policy, got nil")
	}
	if loaded.JobID != jobID {
		t.Fatalf("expected loaded.JobID=%q, got %q", jobID, loaded.JobID)
	}
	if loaded.SchemaVersion != executionPolicySchemaVersion {
		t.Fatalf("expected loaded.SchemaVersion=%q, got %q", executionPolicySchemaVersion, loaded.SchemaVersion)
	}
	if loaded.DefaultAction != decisionExecute {
		t.Fatalf("expected loaded.DefaultAction=%q, got %q", decisionExecute, loaded.DefaultAction)
	}
	if len(loaded.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(loaded.Rules))
	}
	if loaded.Rules[0].Type != "dependency_check" {
		t.Fatalf("expected rule.type=%q, got %q", "dependency_check", loaded.Rules[0].Type)
	}
	if loaded.Rules[0].Action != decisionDefer {
		t.Fatalf("expected rule.action=%q, got %q", decisionDefer, loaded.Rules[0].Action)
	}
	depsAny, ok := loaded.Rules[0].Condition[objects.FieldKeyDependencies]
	if !ok {
		t.Fatalf("expected condition key %q", "dependencies")
	}
	depsSlice, ok := depsAny.([]any)
	if !ok {
		// yaml.v3 sometimes produces []interface{} (aka []any) or []string depending on types,
		// so handle common variants.
		switch v := depsAny.(type) {
		case []string:
			depsSlice = make([]any, 0, len(v))
			for _, s := range v {
				depsSlice = append(depsSlice, s)
			}
		default:
			t.Fatalf("expected dependencies to be slice, got %T", depsAny)
		}
	}
	if len(depsSlice) != 1 || depsSlice[0] != "SCH-9041-2" {
		t.Fatalf("expected dependencies=[%q], got %v", "SCH-9041-2", depsSlice)
	}
}

func TestCRIT9041_PolicyEngine_LoadPolicy_IgnoresUnknownFieldsAndDefaultsSchemaVersion(t *testing.T) {
	t.Parallel()

	tmpRoot := t.TempDir()
	reg := NewJobStateRegistry(tmpRoot)
	pe := NewPolicyEngine(reg, tmpRoot, nil)

	jobID := "SCH-9041-unknown"
	path := pe.(*PolicyEngine).policyFilePath(jobID)
	if path == emptyValue {
		t.Fatalf("expected non-empty policy file path")
	}
	if err := fileutil.EnsureDir(filepath.Dir(path)); err != nil {
		t.Fatalf("mkdir policies dir failed: %v", err)
	}

	// Includes:
	// - unknown top-level field: extra_top
	// - unknown rule.type: new_rule_type (phase-1 doesn't evaluate it anyway)
	// - no schema_version at all (LoadPolicy should default it)
	yml := `job_id: "SCH-9041-unknown"
default_action: "execute"
extra_top:
  foo: "bar"
rules:
  - type: "new_rule_type"
    action: "skip"
    reason: "future_rule"
    condition:
      arbitrary_field: 123
`
	if err := fileutil.WriteSecureFile(path, []byte(yml)); err != nil {
		t.Fatalf("write policy failed: %v", err)
	}

	loaded, err := pe.(*PolicyEngine).LoadPolicy(jobID)
	if err != nil {
		t.Fatalf("LoadPolicy failed: %v", err)
	}
	if loaded == nil {
		t.Fatalf("expected loaded policy, got nil")
	}
	if loaded.SchemaVersion != executionPolicySchemaVersion {
		t.Fatalf("expected SchemaVersion default=%q, got %q", executionPolicySchemaVersion, loaded.SchemaVersion)
	}
	if loaded.JobID != jobID {
		t.Fatalf("expected loaded.JobID=%q, got %q", jobID, loaded.JobID)
	}
	if len(loaded.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(loaded.Rules))
	}
	if loaded.Rules[0].Type != "new_rule_type" {
		t.Fatalf("expected rule.type=%q, got %q", "new_rule_type", loaded.Rules[0].Type)
	}
	if loaded.Rules[0].Action != "skip" {
		t.Fatalf("expected rule.action=%q, got %q", "skip", loaded.Rules[0].Action)
	}
	if v, ok := loaded.Rules[0].Condition["arbitrary_field"]; !ok {
		t.Fatalf("expected arbitrary_field in rule.condition")
	} else if fmt.Sprint(v) != "123" {
		t.Fatalf("expected arbitrary_field=123, got %v (%T)", v, v)
	}
}

func TestCRIT9042_PolicyEngine_Evaluate_UsesPolicyDefaultActionSkipWhenNoState(t *testing.T) {
	t.Parallel()

	tmpRoot := t.TempDir()
	reg := NewJobStateRegistry(tmpRoot)
	pe := NewPolicyEngine(reg, tmpRoot, nil)

	jobID := "SCH-9042-1"
	if err := pe.(*PolicyEngine).SavePolicy(&ExecutionPolicy{
		JobID:         jobID,
		DefaultAction: decisionSkip,
		Rules:         []PolicyRule{},
	}); err != nil {
		t.Fatalf("SavePolicy failed: %v", err)
	}

	decision, err := pe.(*PolicyEngine).Evaluate(jobID, &ScheduledJob{ID: jobID})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}
	if decision.Action != decisionSkip {
		t.Fatalf("expected action=%q, got %q", decisionSkip, decision.Action)
	}
	if decision.Reason == emptyValue {
		t.Fatalf("expected non-empty Reason")
	}
}

func TestPolicyEngine_Evaluate_SkipWindowSkipsMatchingJob(t *testing.T) {
	t.Parallel()
	tmpRoot := t.TempDir()
	reg := NewJobStateRegistry(tmpRoot)
	pe := NewPolicyEngine(reg, tmpRoot, nil)
	schedDir := filepath.Join(tmpRoot, paths.ProjectDataDir, paths.SchedulerDir)
	sw := &SchedulerSkipWindow{
		Until:    time.Now().UTC().Add(30 * time.Minute),
		JobTypes: []string{"convergence_session_tick"},
	}
	if err := SaveSchedulerSkipWindow(schedDir, sw); err != nil {
		t.Fatal(err)
	}
	decision, err := pe.(*PolicyEngine).Evaluate("SCH-cvs", &ScheduledJob{
		ID:          "SCH-cvs",
		JobType:     "convergence_session_tick",
		TriggerType: TriggerTypeTimer,
		Title:       "tick",
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if decision.Action != decisionSkip {
		t.Fatalf("got action=%q reason=%q", decision.Action, decision.Reason)
	}
	_ = ClearSchedulerSkipWindow(schedDir)
}

func TestPolicyEngine_Evaluate_BlocksBacklogItemWithoutQAGate(t *testing.T) {
	tmpDir := t.TempDir()
	reg := NewJobStateRegistry(tmpDir)

	// Create policy engine without storage (causes QA Gate to fail)
	pe := NewPolicyEngine(reg, tmpDir, nil)

	jobID := "BLI-123"
	job := &ScheduledJob{ID: jobID}

	dec, err := pe.(*PolicyEngine).Evaluate(jobID, job)
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	if dec.Action != decisionSkip {
		t.Errorf("expected skip action, got %s", dec.Action)
	}

	if dec.Reason == "" || !contains(dec.Reason, "QA Gate Fail") {
		t.Errorf("expected QA Gate failure reason, got %s", dec.Reason)
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr) // Simple helper for test
}
