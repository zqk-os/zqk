package validation

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/pipeline"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestGoValidator_PriorityPlanSealRequiresPlannedChild(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	lifecyclesDir := filepath.Join(tmpDir, paths.ProcessInternalLifecyclesDir)
	if err := fileutil.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	lifecycleContent := `object_type: priority_plan
statuses:
  - value: grooming
    role: grooming
  - value: active
    role: shovel_ready
transitions:
  - from: grooming
    to: active
    manual: true
    preconditions:
      - at least one planned backlog_item references this plan via priority_plan_ref
`
	if err := fileutil.WriteFile(filepath.Join(lifecyclesDir, "priority_plan_lifecycle.yaml"), []byte(lifecycleContent), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	planID := "PRI-SEAL-1"
	gv := NewGoValidatorWithLoaders(nil, objects.NewLifecycleLoader(lifecyclesDir))
	obj := map[string]any{
		objects.FieldKeyID:     planID,
		objects.FieldKeyKind:   objects.KindPriorityPlan,
		objects.FieldKeyStatus: objects.ObjectStatusActive,
	}

	emptyOpts := &ValidationOptions{
		CurrentState:       objects.ObjectStatusGrooming,
		ValidateLifecycle:  true,
		DependentsLookup:   func(string) []string { return []string{} },
		ObjectStatusLookup: func(string) (string, error) { return objects.ObjectStatusPlanned, nil },
	}
	errs, _ := gv.validateLifecycleState(t.Context(), objects.KindPriorityPlan, objects.ObjectStatusActive, objects.ObjectStatusGrooming, obj, emptyOpts)
	if len(errs) == 0 {
		t.Fatal("grooming→active with zero planned children must refuse")
	}
	got := false
	for _, e := range errs {
		if strings.Contains(e.Message, PrecondReadyBacklogReferencesPlan) {
			got = true
			break
		}
	}
	if !got {
		t.Fatalf("want membership token in errors, got %v", errs)
	}

	readyOpts := &ValidationOptions{
		CurrentState:      objects.ObjectStatusGrooming,
		ValidateLifecycle: true,
		DependentsLookup: func(string) []string {
			return []string{"BLI-PLANNED-1"}
		},
		ObjectStatusLookup: func(string) (string, error) {
			return objects.ObjectStatusPlanned, nil
		},
	}
	errs, _ = gv.validateLifecycleState(t.Context(), objects.KindPriorityPlan, objects.ObjectStatusActive, objects.ObjectStatusGrooming, obj, readyOpts)
	for _, e := range errs {
		if strings.Contains(e.Message, PrecondReadyBacklogReferencesPlan) {
			t.Fatalf("planned child must satisfy seal exam; errors=%v", errs)
		}
	}
}

func TestEvaluatePrecondition_ReadyBacklogTokenIsRecognized(t *testing.T) {
	t.Parallel()
	gv := NewGoValidator()
	obj := map[string]any{objects.FieldKeyID: "PRI-SEAL-2"}
	opts := &ValidationOptions{
		DependentsLookup:   func(string) []string { return []string{} },
		ObjectStatusLookup: func(string) (string, error) { return objects.ObjectStatusPlanned, nil },
	}
	met, recognized := gv.evaluatePrecondition(PrecondReadyBacklogReferencesPlan, obj, opts)
	if !recognized {
		t.Fatal("membership token must be recognized")
	}
	if met {
		t.Fatal("zero planned children must fail the token")
	}
}

func TestEvaluatePrecondition_UnrecognizedFailsClosed(t *testing.T) {
	t.Parallel()
	gv := NewGoValidator()
	met, recognized := gv.evaluatePrecondition("the overlay compiler has compiled this sentence", map[string]any{}, nil)
	if recognized {
		t.Fatal("prose without a token must be unrecognized")
	}
	if met {
		t.Fatal("unrecognized prose must fail closed under overlay DSL")
	}
}

func TestEvaluatePrecondition_PriorityPlanValidated(t *testing.T) {
	t.Parallel()
	gv := NewGoValidator()

	met, recognized := gv.evaluatePrecondition(PrecondPriorityPlanValidated, map[string]any{}, nil)
	if !recognized {
		t.Fatal("validated token must be recognized")
	}
	if met {
		t.Fatal("empty plan must fail validated")
	}

	plan := map[string]any{
		objects.FieldKeyTitle: "CEF Round 23 — package directory names",
	}
	met, _ = gv.evaluatePrecondition("Priority plan validated", plan, nil)
	if met {
		t.Fatal("title without workstream lane must fail")
	}

	plan[objects.FieldKeyWorkstreamRefs] = []string{"WS-CEF-ARCHITECTURE"}
	met, recognized = gv.evaluatePrecondition("Priority plan validated", plan, nil)
	if !recognized {
		t.Fatal("YAML casing must still be recognized")
	}
	if !met {
		t.Fatal("inherited title plus workstream_refs must pass")
	}

	singular := map[string]any{
		objects.FieldKeyDescription:   "intake notes only",
		objects.FieldKeyWorkstreamRef: "WS-CEF-ARCHITECTURE",
	}
	met, _ = gv.evaluatePrecondition(PrecondPriorityPlanValidated, singular, nil)
	if !met {
		t.Fatal("description plus singular workstream_ref must pass")
	}
}

func TestEvaluatePrecondition_WorkflowConstraintsIfSet(t *testing.T) {
	t.Parallel()
	gv := NewGoValidator()

	met, recognized := gv.evaluatePrecondition(PrecondWorkflowConstraintsIfSet, map[string]any{}, nil)
	if !recognized {
		t.Fatal("workflow token must be recognized")
	}
	if !met {
		t.Fatal("unset workflow_ref must be vacuous true")
	}

	plan := map[string]any{objects.FieldKeyWorkflowRef: "WFL-MISSING-1"}
	met, recognized = gv.evaluatePrecondition(PrecondWorkflowConstraintsIfSet, plan, nil)
	if !recognized {
		t.Fatal("workflow token must be recognized when ref is set")
	}
	if met {
		t.Fatal("set workflow_ref without ObjectLookup must fail closed")
	}

	opts := &ValidationOptions{
		ObjectLookup: func(id string) (map[string]any, error) {
			if id != "WFL-OK-1" {
				return nil, fmt.Errorf("missing workflow %s", id)
			}
			return map[string]any{
				objects.FieldKeyID:      id,
				objects.FieldKeyKind:    objects.KindWorkflow,
				objects.FieldKeyEnabled: true,
			}, nil
		},
	}
	plan[objects.FieldKeyWorkflowRef] = "WFL-OK-1"
	met, _ = gv.evaluatePrecondition(PrecondWorkflowConstraintsIfSet, plan, opts)
	if !met {
		t.Fatal("enabled workflow must pass")
	}

	optsDisabled := &ValidationOptions{
		ObjectLookup: func(string) (map[string]any, error) {
			return map[string]any{
				objects.FieldKeyKind:    objects.KindWorkflow,
				objects.FieldKeyEnabled: false,
			}, nil
		},
	}
	met, _ = gv.evaluatePrecondition(PrecondWorkflowConstraintsIfSet, plan, optsDisabled)
	if met {
		t.Fatal("disabled workflow must fail")
	}
}

func TestPreconditionPipeline_ExplicitRulesBeforeActiveHeuristic(t *testing.T) {
	t.Parallel()
	names := precondDecideRuleNames()
	ref := indexOfStage(names, "ref_status_matrix")
	active := indexOfStage(names, "active_ref")
	if ref < 0 || active < 0 {
		t.Fatalf("missing stages: %v", names)
	}
	if ref >= active {
		t.Fatalf("ref_status_matrix must precede active_ref so a token mentioning active cannot widen a specific rule; got %v", names)
	}
	membership := indexOfStage(names, "ready_backlog_references_plan")
	if membership < 0 || membership >= active {
		t.Fatalf("exact membership tokens must precede active_ref; got %v", names)
	}
}

func TestLifecyclePreconditionPipeline_CanonicalStageOrder(t *testing.T) {
	t.Parallel()
	got := lifecyclePreconditionStageOrder()
	want := []string{pipeline.StageIngest, pipeline.StageNormalize, pipeline.StageDecide, pipeline.StageFinalize}
	if len(got) != len(want) {
		t.Fatalf("stage order %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("stage[%d]=%s want %s (DECIDE rules are not AddStage names)", i, got[i], want[i])
		}
	}
}

func TestDispatchPrecondition_RecordsPipelineOutcome(t *testing.T) {
	t.Parallel()
	gv := NewGoValidator()
	pctx, met := gv.runLifecyclePreconditionPipeline(PrecondPriorityPlanValidated, map[string]any{}, nil, nil)
	if met {
		t.Fatal("empty plan must fail validated")
	}
	if pctx == nil || pctx.Outcome == nil {
		t.Fatal("pipeline context outcome missing")
	}
	if got := pctx.Outcome[pipeline.OutcomeKeyPlan]; got != "priority_plan_validated" {
		t.Fatalf("plan=%v want priority_plan_validated", got)
	}
	if ok, _ := pctx.Outcome[pipeline.OutcomeKeyLifecycleOk].(bool); ok {
		t.Fatal("lifecycle_ok must be false for empty plan")
	}
	if rec, _ := pctx.Outcome[pipeline.OutcomeKeyValidationSuccess].(bool); !rec {
		t.Fatal("token must be recognized")
	}
	if done, _ := pctx.Outcome[pipeline.OutcomeKeyFinalizeDone].(bool); !done {
		t.Fatal("FINALIZE must record finalize_done")
	}
}

func TestPreconditionPipeline_UnrecognizedFailsClosed(t *testing.T) {
	t.Parallel()
	gv := NewGoValidator()
	met := gv.dispatchPrecondition("the overlay compiler has compiled this sentence", map[string]any{}, nil, nil)
	if met {
		t.Fatal("unrecognized prose must fail closed under overlay DSL")
	}
	recognized := true
	met = gv.dispatchPrecondition("the overlay compiler has compiled this sentence", map[string]any{}, nil, &recognized)
	if recognized || met {
		t.Fatalf("recognized=%v met=%v; want recognized=false met=false", recognized, met)
	}
}

func indexOfStage(names []string, want string) int {
	for i, n := range names {
		if n == want {
			return i
		}
	}
	return -1
}

func TestFormatUnmetPreconditionDetail_PriorityPlanValidated(t *testing.T) {
	t.Parallel()

	// 1. Missing title and description, has workstream lane
	objNoTitle := map[string]any{
		objects.FieldKeyWorkstreamRefs: []string{"WS-ARCH"},
	}
	detail := FormatUnmetPreconditionDetail(PrecondPriorityPlanValidated, objNoTitle)
	expected := "priority plan validated: 'title' or 'description' written identity must be specified"
	if detail != expected {
		t.Fatalf("got %q, want %q", detail, expected)
	}

	// 2. Has title, missing workstream lane
	objNoLane := map[string]any{
		objects.FieldKeyTitle: "My Priority Plan",
	}
	detail = FormatUnmetPreconditionDetail(PrecondPriorityPlanValidated, objNoLane)
	expected = "priority plan validated: 'workstream_refs' or 'workstream_ref' must be specified"
	if detail != expected {
		t.Fatalf("got %q, want %q", detail, expected)
	}

	// 3. Missing both written identity and workstream lane
	objEmpty := map[string]any{}
	detail = FormatUnmetPreconditionDetail(PrecondPriorityPlanValidated, objEmpty)
	expected = "priority plan validated: 'title' or 'description' written identity must be specified; 'workstream_refs' or 'workstream_ref' must be specified"
	if detail != expected {
		t.Fatalf("got %q, want %q", detail, expected)
	}

	// 4. nil object
	detail = FormatUnmetPreconditionDetail(PrecondPriorityPlanValidated, nil)
	if detail != expected {
		t.Fatalf("got %q, want %q", detail, expected)
	}
}

func TestFormatUnmetPreconditionDetail_GitMutationEvidence(t *testing.T) {
	t.Parallel()

	// 1. With BLI ID
	objWithID := map[string]any{
		objects.FieldKeyID: "BLI-1234",
	}
	detail := FormatUnmetPreconditionDetail(PrecondCommitHashesGitMutationEvidence, objWithID)
	expected := "commit_hashes have git mutation evidence for this backlog_item (ensure at least one cited commit is a non-merge commit reachable from branch_name, changes at least one file outside .zqk/process/, and mentions BLI-1234 in its commit message (recommended subject tag: \"[BLI-1234] <summary>\"))"
	if detail != expected {
		t.Fatalf("got %q, want %q", detail, expected)
	}

	// 2. Without BLI ID (empty map)
	detail = FormatUnmetPreconditionDetail(PrecondCommitHashesGitMutationEvidence, map[string]any{})
	expected = "commit_hashes have git mutation evidence for this backlog_item (ensure at least one cited commit is a non-merge commit reachable from branch_name, changes at least one file outside .zqk/process/, and mentions BLI-xxx in its commit message (recommended subject tag: \"[BLI-xxx] <summary>\"))"
	if detail != expected {
		t.Fatalf("got %q, want %q", detail, expected)
	}

	// 3. nil object
	detail = FormatUnmetPreconditionDetail(PrecondCommitHashesGitMutationEvidence, nil)
	if detail != expected {
		t.Fatalf("got %q, want %q", detail, expected)
	}
}

func TestValidateLifecycleState_DetailedPreconditionErrors(t *testing.T) {
	t.Parallel()
	gv := NewGoValidator()

	t.Run("PriorityPlanMissingWorkstreamLane", func(t *testing.T) {
		obj := map[string]any{
			objects.FieldKeyID:     "PRI-LIFECYCLE-1",
			objects.FieldKeyKind:   objects.KindPriorityPlan,
			objects.FieldKeyStatus: objects.ObjectStatusActive,
			objects.FieldKeyTitle:  "Valid Plan Title",
			"persona_refs":         []string{"PER-ENG"},
		}
		opts := &ValidationOptions{
			CurrentState:      objects.ObjectStatusGrooming,
			ValidateLifecycle: true,
			DependentsLookup: func(string) []string {
				return []string{"BLI-CHILD-1"}
			},
			ObjectStatusLookup: func(string) (string, error) {
				return objects.ObjectStatusPlanned, nil
			},
		}
		errs, _ := gv.validateLifecycleState(t.Context(), objects.KindPriorityPlan, objects.ObjectStatusActive, objects.ObjectStatusGrooming, obj, opts)
		var found bool
		for _, e := range errs {
			if strings.Contains(e.Message, "priority plan validated: 'workstream_refs' or 'workstream_ref' must be specified") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected detailed workstream lane error, got errs: %v", errs)
		}
	})

	t.Run("PriorityPlanMissingWrittenIdentity", func(t *testing.T) {
		obj := map[string]any{
			objects.FieldKeyID:             "PRI-LIFECYCLE-2",
			objects.FieldKeyKind:           objects.KindPriorityPlan,
			objects.FieldKeyStatus:         objects.ObjectStatusActive,
			objects.FieldKeyWorkstreamRefs: []string{"WS-ENG"},
			"persona_refs":                 []string{"PER-ENG"},
		}
		opts := &ValidationOptions{
			CurrentState:      objects.ObjectStatusGrooming,
			ValidateLifecycle: true,
			DependentsLookup: func(string) []string {
				return []string{"BLI-CHILD-1"}
			},
			ObjectStatusLookup: func(string) (string, error) {
				return objects.ObjectStatusPlanned, nil
			},
		}
		errs, _ := gv.validateLifecycleState(t.Context(), objects.KindPriorityPlan, objects.ObjectStatusActive, objects.ObjectStatusGrooming, obj, opts)
		var found bool
		for _, e := range errs {
			if strings.Contains(e.Message, "priority plan validated: 'title' or 'description' written identity must be specified") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected detailed written identity error, got errs: %v", errs)
		}
	})

	t.Run("BacklogItemGitMutationEvidence", func(t *testing.T) {
		obj := map[string]any{
			objects.FieldKeyID:     "BLI-TEST-999",
			objects.FieldKeyKind:   objects.KindBacklogItem,
			objects.FieldKeyStatus: objects.ObjectStatusComplete,
			"criteria_refs":        []string{"CRIT-1"},
		}
		opts := &ValidationOptions{
			CurrentState:      objects.ObjectStatusInProgress,
			ValidateLifecycle: true,
			ObjectStatusLookup: func(string) (string, error) {
				return objects.ObjectStatusValidated, nil
			},
		}
		errs, _ := gv.validateLifecycleState(t.Context(), objects.KindBacklogItem, objects.ObjectStatusComplete, objects.ObjectStatusInProgress, obj, opts)
		var found bool
		wantSub := "commit_hashes have git mutation evidence for this backlog_item (ensure at least one cited commit is a non-merge commit reachable from branch_name, changes at least one file outside .zqk/process/, and mentions BLI-TEST-999 in its commit message (recommended subject tag: \"[BLI-TEST-999] <summary>\"); rejected: "
		for _, e := range errs {
			if strings.Contains(e.Message, wantSub) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected git mutation evidence error with tag format, got errs: %v", errs)
		}
	})
}

func TestFormatUnmetPreconditionDetail_GitMutationEvidencePlanAltAndIdempotent(t *testing.T) {
	t.Parallel()
	obj := map[string]any{
		objects.FieldKeyID:              "BLI-77",
		objects.FieldKeyPriorityPlanRef: "PRI-9",
	}
	detail := FormatUnmetPreconditionDetail(PrecondCommitRefsGitMutationEvidence, obj)
	for _, want := range []string{"mentions BLI-77 (or its priority_plan_ref PRI-9)", `"[BLI-77] <summary>"`} {
		if !strings.Contains(detail, want) {
			t.Fatalf("detail %q missing %q", detail, want)
		}
	}
	// Already-enriched messages must not be double-annotated.
	if again := FormatUnmetPreconditionDetail(detail, obj); again != detail {
		t.Fatalf("expected idempotent enrichment, got %q", again)
	}
	// Unrelated preconditions pass through untouched.
	if got := FormatUnmetPreconditionDetail(PrecondCommitHashesNotEmpty, obj); got != PrecondCommitHashesNotEmpty {
		t.Fatalf("unrelated precondition mutated: %q", got)
	}
}

func TestFormatUnmetPreconditionDetailForOptions_SurfacesPerCommitRejection(t *testing.T) {
	t.Parallel()
	gv := NewGoValidator()
	root := t.TempDir()
	obj := map[string]any{
		objects.FieldKeyID:           "BLI-REJECT-1",
		objects.FieldKeyCommitHashes: []string{"deadbeefcafe"},
	}
	opts := &ValidationOptions{ProjectRoot: root, CurrentState: objects.ObjectStatusInProgress}
	detail := gv.formatUnmetPreconditionDetailForOptions(PrecondCommitHashesGitMutationEvidence, obj, opts)
	for _, want := range []string{`"[BLI-REJECT-1] <summary>"`, "; rejected: ", `commit_hashes "deadbeefcafe" is not a resolvable git commit`} {
		if !strings.Contains(detail, want) {
			t.Fatalf("detail %q missing %q", detail, want)
		}
	}
	// Empty commit_hashes names the concrete reason too.
	delete(obj, objects.FieldKeyCommitHashes)
	detail = gv.formatUnmetPreconditionDetailForOptions(PrecondCommitHashesGitMutationEvidence, obj, opts)
	if !strings.Contains(detail, "; rejected: commit_hashes is empty") {
		t.Fatalf("expected empty-hashes rejection, got %q", detail)
	}
}
