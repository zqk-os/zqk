package validation

import (
	"path/filepath"
	"strings"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestGoValidator_BreakGlassStillEnforcesCriteriaOnComplete(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	lifecyclesDir := filepath.Join(tmpDir, paths.ProcessInternalLifecyclesDir)
	if err := fileutil.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	lifecycleFile := filepath.Join(lifecyclesDir, "backlog_item_lifecycle.yaml")
	lifecycleContent := `object_type: backlog_item
statuses:
  - value: in_progress
  - value: complete
    terminal: true
    preconditions:
      - all linked criteria_refs are validated or complete (archived-only lineage also satisfies; never delete archived CRITs to clear this gate)
transitions:
  - from: in_progress
    to: complete
    auto: true
    preconditions:
      - all linked criteria_refs are validated or complete (archived-only lineage also satisfies; never delete archived CRITs to clear this gate)
`
	if err := fileutil.WriteFile(lifecycleFile, []byte(lifecycleContent), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	gv := NewGoValidatorWithLoaders(nil, objects.NewLifecycleLoader(lifecyclesDir))
	obj := map[string]any{
		objects.FieldKeyID:           "BLI-TEST-1",
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyStatus:       objects.ObjectStatusComplete,
		objects.FieldKeyCriteriaRefs: []any{"CRIT-1"},
	}
	opts := &ValidationOptions{
		CurrentState:      objects.ObjectStatusInProgress,
		ValidateLifecycle: true,
		ObjectStatusLookup: func(string) (string, error) {
			return objects.ObjectStatusAwaitingVerification, nil
		},
	}
	ctx := pkgctx.WithLifecycleBreakGlass(t.Context(), "lifecycle updater auto-complete transition")
	errs, _ := gv.validateLifecycleState(ctx, objects.KindBacklogItem, objects.ObjectStatusComplete, objects.ObjectStatusInProgress, obj, opts)
	gotCriteria := false
	for _, e := range errs {
		if strings.Contains(e.Message, "criteria_refs") {
			gotCriteria = true
			break
		}
	}
	if !gotCriteria {
		t.Fatalf("break-glass must still fail complete when CRITs are awaiting_verification; errors=%v", errs)
	}
}

func TestGoValidator_TrustedShockwaveStillEnforcesPlanChildrenHold(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	lifecyclesDir := filepath.Join(tmpDir, paths.ProcessInternalLifecyclesDir)
	if err := fileutil.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	lifecycleFile := filepath.Join(lifecyclesDir, "priority_plan_lifecycle.yaml")
	lifecycleContent := `object_type: priority_plan
statuses:
  - value: in_progress
  - value: complete
    terminal: true
    preconditions:
      - all linked backlog_items referencing this plan are terminal
transitions:
  - from: in_progress
    to: complete
    auto: true
    preconditions:
      - all linked backlog_items referencing this plan are terminal
`
	if err := fileutil.WriteFile(lifecycleFile, []byte(lifecycleContent), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	planID := "PRI-TEST-1"
	gv := NewGoValidatorWithLoaders(nil, objects.NewLifecycleLoader(lifecyclesDir))
	obj := map[string]any{
		objects.FieldKeyID:     planID,
		objects.FieldKeyKind:   objects.KindPriorityPlan,
		objects.FieldKeyStatus: objects.ObjectStatusComplete,
	}
	opts := &ValidationOptions{
		CurrentState:      objects.ObjectStatusInProgress,
		ValidateLifecycle: true,
		DependentsLookup: func(string) []string {
			return []string{"BLI-OPEN-1"}
		},
		ObjectStatusLookup: func(string) (string, error) {
			return objects.ObjectStatusInProgress, nil
		},
	}
	ctx := pkgctx.WithTrustedLifecycleEvent(t.Context(), pkgctx.TrustedLifecycleEvent{
		EventID:   "EVT-1",
		TargetID:  planID,
		TriggerID: "BLI-OPEN-1",
		FromState: objects.ObjectStatusInProgress,
		ToState:   objects.ObjectStatusComplete,
	})
	errs, _ := gv.validateLifecycleState(ctx, objects.KindPriorityPlan, objects.ObjectStatusComplete, objects.ObjectStatusInProgress, obj, opts)
	gotHold := false
	for _, e := range errs {
		if strings.Contains(strings.ToLower(e.Message), "terminal") || strings.Contains(e.Message, "backlog_item") {
			gotHold = true
			break
		}
	}
	if !gotHold {
		t.Fatalf("trusted shockwave must still fail plan complete with open children; errors=%v", errs)
	}
}

func TestGoValidator_ShippedBacklogCompleteRefusesInProgressCriteria(t *testing.T) {
	t.Parallel()
	// TRACK: BLI-CEF-POL007-FMT-001 — live YAML must refuse complete while CRITs are in_progress
	loader := objects.NewLifecycleLoader(filepath.Join("..", "..", paths.ProcessInternalLifecyclesDir))
	gv := NewGoValidatorWithLoaders(nil, loader)
	obj := map[string]any{
		objects.FieldKeyID:           "BLI-CEF-POL007-FMT-001",
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyStatus:       objects.ObjectStatusComplete,
		objects.FieldKeyCriteriaRefs: []any{"CRIT-CEF-FORBIDIGO-ZERO-PKG-001"},
		objects.FieldKeyCommitHashes: []any{"754e299db9"},
	}
	opts := &ValidationOptions{
		CurrentState:      objects.ObjectStatusInProgress,
		ValidateLifecycle: true,
		ObjectStatusLookup: func(string) (string, error) {
			return objects.ObjectStatusInProgress, nil
		},
	}
	errs, _ := gv.validateLifecycleState(t.Context(), objects.KindBacklogItem, objects.ObjectStatusComplete, objects.ObjectStatusInProgress, obj, opts)
	gotCriteria := false
	for _, e := range errs {
		if strings.Contains(e.Message, "criteria_refs") {
			gotCriteria = true
			break
		}
	}
	if !gotCriteria {
		t.Fatalf("shipped backlog complete hop must fail when linked CRIT is in_progress; errors=%v", errs)
	}
}
