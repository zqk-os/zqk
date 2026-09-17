package lifecycle

import (
	"context"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/coordination"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
)

// Compiled Plane C matchers: YAML on_dependent_status interpreted by
// shockwaveUpdatesFromTransitions. Rubric: child-status hops live on C+D;
// the listener kind's YAML is the rule table, not a Go specialty.
func TestCompiledShockwaveYAMLMatchers(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		kind        string
		from        string
		triggerKind string
		toState     string
		wantStatus  string
		wantOK      bool
	}{
		// priority_plan (gantt_column)
		{"pri_active_locks", objects.KindPriorityPlan, statusActive, objects.KindBacklogItem, statusInProgress, statusInProgress, true},
		{"pri_grooming_no_skip_seal", objects.KindPriorityPlan, statusGrooming, objects.KindBacklogItem, statusInProgress, "", false},
		{"pri_paused_relocks", objects.KindPriorityPlan, statusPaused, objects.KindBacklogItem, statusInProgress, statusInProgress, true},
		{"pri_blocked_relocks", objects.KindPriorityPlan, statusBlocked, objects.KindBacklogItem, statusInProgress, statusInProgress, true},
		{"pri_already_locked", objects.KindPriorityPlan, statusInProgress, objects.KindBacklogItem, statusInProgress, "", false},
		{"pri_planned_child_no_lock", objects.KindPriorityPlan, statusActive, objects.KindBacklogItem, "planned", "", false},
		{"pri_complete_child_no_lock", objects.KindPriorityPlan, statusActive, objects.KindBacklogItem, statusComplete, "", false},
		{"pri_in_progress_exploring_no_yaml", objects.KindPriorityPlan, statusInProgress, objects.KindBacklogItem, statusExploring, "", false},
		{"pri_active_validated_unseals", objects.KindPriorityPlan, statusActive, objects.KindBacklogItem, statusValidated, statusGrooming, true},
		{"pri_paused_exploring_unseals", objects.KindPriorityPlan, statusPaused, objects.KindBacklogItem, statusExploring, statusGrooming, true},
		{"pri_blocked_validated_unseals", objects.KindPriorityPlan, statusBlocked, objects.KindBacklogItem, statusValidated, statusGrooming, true},
		{"pri_wrong_trigger", objects.KindPriorityPlan, statusActive, objects.KindMilestone, statusInProgress, "", false},

		// milestone (work_unit compiled target)
		{"mil_not_started_locks", objects.KindMilestone, objects.ObjectStatusNotStarted, objects.KindBacklogItem, statusInProgress, statusInProgress, true},
		{"mil_already_locked", objects.KindMilestone, statusInProgress, objects.KindBacklogItem, statusInProgress, "", false},
		{"mil_planned_child_no_lock", objects.KindMilestone, objects.ObjectStatusNotStarted, objects.KindBacklogItem, "planned", "", false},
		{"mil_wrong_trigger", objects.KindMilestone, objects.ObjectStatusNotStarted, objects.KindAgentTask, statusInProgress, "", false},

		// goal (gantt_lane compiled target; active is executing occupancy)
		{"goal_proposed_activates", objects.KindGoal, objects.ObjectStatusProposed, objects.KindBacklogItem, statusInProgress, statusActive, true},
		{"goal_already_active", objects.KindGoal, statusActive, objects.KindBacklogItem, statusInProgress, "", false},
		{"goal_wrong_trigger", objects.KindGoal, objects.ObjectStatusProposed, objects.KindWorkstream, statusInProgress, "", false},

		// criteria (predicate parent-lock). Destination is in_progress, never validated.
		{"crit_shovel_ready_locks", objects.KindCriteria, objects.ObjectStatusAwaitingVerification, objects.KindBacklogItem, statusInProgress, statusInProgress, true},
		{"crit_already_locked", objects.KindCriteria, statusInProgress, objects.KindBacklogItem, statusInProgress, "", false},
		{"crit_validated_noop", objects.KindCriteria, statusValidated, objects.KindBacklogItem, statusInProgress, "", false},
		{"crit_complete_noop", objects.KindCriteria, statusComplete, objects.KindBacklogItem, statusInProgress, "", false},
		{"crit_archived_noop", objects.KindCriteria, statusArchived, objects.KindBacklogItem, statusInProgress, "", false},
		{"crit_blocked_noop", objects.KindCriteria, statusBlocked, objects.KindBacklogItem, statusInProgress, "", false},
		{"crit_bli_planned_noop", objects.KindCriteria, objects.ObjectStatusAwaitingVerification, objects.KindBacklogItem, "planned", "", false},
		{"crit_bli_complete_noop", objects.KindCriteria, objects.ObjectStatusAwaitingVerification, objects.KindBacklogItem, statusComplete, "", false},
		{"crit_wrong_trigger", objects.KindCriteria, objects.ObjectStatusAwaitingVerification, objects.KindMilestone, statusInProgress, "", false},

		// agent_task YAML declares parent-terminal → archived, but the kind is not
		// status_reactive so Plane C no-ops (see apply test below). Matcher still exists.
		{"atk_parent_complete_matcher", objects.KindAgentTask, statusInProgress, objects.KindBacklogItem, statusComplete, statusArchived, true},
		{"atk_parent_archived_matcher", objects.KindAgentTask, statusInProgress, objects.KindBacklogItem, statusArchived, statusArchived, true},
		{"atk_parent_in_progress_noop", objects.KindAgentTask, statusInProgress, objects.KindBacklogItem, statusInProgress, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			transitions := loadKindShockwaveTransitions(t, tc.kind)
			u, ok := shockwaveUpdatesFromTransitions(transitions, tc.from, tc.triggerKind, tc.toState)
			if ok != tc.wantOK {
				t.Fatalf("ok=%v want %v updates=%v", ok, tc.wantOK, u)
			}
			if !tc.wantOK {
				if status, _ := u[objects.FieldKeyStatus].(string); status == statusValidated {
					t.Fatalf("compiled matcher must never target validated, got %v", u)
				}
				return
			}
			got, _ := u[objects.FieldKeyStatus].(string)
			if got != tc.wantStatus {
				t.Fatalf("status=%q want %q", got, tc.wantStatus)
			}
			if got == statusValidated {
				t.Fatal("compiled matcher must never target validated")
			}
		})
	}
}

func TestDependencyRefTargetIDsIncludesCriteriaRefs(t *testing.T) {
	t.Parallel()
	got := dependencyRefTargetIDs("BLI-1", map[string]any{
		objects.FieldKeyCriteriaRefs: []any{"CRIT-1", "CRIT-2"},
	})
	want := map[string]bool{"CRIT-1": true, "CRIT-2": true}
	if len(got) != 2 {
		t.Fatalf("target IDs = %#v, want CRIT-1 and CRIT-2", got)
	}
	for _, id := range got {
		if !want[id] {
			t.Fatalf("unexpected target %q in %#v", id, got)
		}
	}
}

func TestDependencyRefTargetIDsEmptyWithoutOutboundRefs(t *testing.T) {
	t.Parallel()
	got := dependencyRefTargetIDs("BLI-1", map[string]any{
		objects.FieldKeyKind:   objects.KindBacklogItem,
		objects.FieldKeyStatus: statusInProgress,
	})
	if len(got) != 0 {
		t.Fatalf("target IDs = %#v, want none", got)
	}
}

func TestDependencyHopMetaTagsCriteriaComposition(t *testing.T) {
	t.Parallel()
	field, role := dependencyHopMeta(objects.KindBacklogItem, "CRIT-1", map[string]any{
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyCriteriaRefs: []any{"CRIT-1"},
	})
	if field != objects.FieldKeyCriteriaRefs {
		t.Fatalf("field = %q, want criteria_refs", field)
	}
	if role != objects.EdgeRoleComposition {
		t.Fatalf("role = %s, want composition", role)
	}
}

func TestApplyDependencyRefEvent_CriteriaParentLockMatrix(t *testing.T) {
	// TRACK: BLI-REDACTED — no t.Parallel: shared Memgraph.
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	logger := logging.NewEventLogger(ctx)

	cases := []struct {
		name        string
		critStatus  string
		triggerKind string
		toState     string
		want        string
	}{
		{"shovel_ready_locks", objects.ObjectStatusAwaitingVerification, objects.KindBacklogItem, statusInProgress, statusInProgress},
		{"already_locked", statusInProgress, objects.KindBacklogItem, statusInProgress, statusInProgress},
		{"validated_stays", statusValidated, objects.KindBacklogItem, statusInProgress, statusValidated},
		{"complete_stays", statusComplete, objects.KindBacklogItem, statusInProgress, statusComplete},
		{"archived_stays", statusArchived, objects.KindBacklogItem, statusInProgress, statusArchived},
		{"blocked_stays", statusBlocked, objects.KindBacklogItem, statusInProgress, statusBlocked},
		{"bli_planned_no_lock", objects.ObjectStatusAwaitingVerification, objects.KindBacklogItem, "planned", objects.ObjectStatusAwaitingVerification},
		{"wrong_kind", objects.ObjectStatusAwaitingVerification, objects.KindMilestone, statusInProgress, objects.ObjectStatusAwaitingVerification},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			critID := fixtureID(t, "CRIT")
			bliID := fixtureID(t, "BLI")
			mustCreateCASVisible(t, realStorage, ctx, secCtx, map[string]any{
				objects.FieldKeyID:       critID,
				objects.FieldKeyKind:     objects.KindCriteria,
				objects.FieldKeyStatus:   tc.critStatus,
				objects.FieldKeyCategory: "acceptance",
			})
			applyDependencyRefEvent(ctx, logger, realStorage, DependencyRefEvent{
				ProjectRoot: t.TempDir(),
				EventID:     "test-crit-lock-" + tc.name + "-" + critID,
				Version:     "2030-01-01T00:00:00.000000001Z",
				TargetID:    critID,
				TriggerID:   bliID,
				TriggerKind: tc.triggerKind,
				FromState:   objects.ObjectStatusPlanned,
				ToState:     tc.toState,
			})
			obj, err := realStorage.Read(ctx, secCtx, critID)
			if err != nil {
				t.Fatalf("read criteria: %v", err)
			}
			got, _ := obj[objects.FieldKeyStatus].(string)
			if got != tc.want {
				t.Errorf("criteria status = %q, want %q", got, tc.want)
			}
			if got == statusValidated && tc.critStatus != statusValidated {
				t.Error("parent-lock must not land on validated")
			}
		})
	}
}

func TestApplyDependencyRefEvent_AgentTaskNotStatusReactive(t *testing.T) {
	// YAML on_dependent_status without status_reactive is exam-only: no hop.
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	atkID := fixtureID(t, "ATK")
	bliID := fixtureID(t, "BLI")

	mustCreateCASVisible(t, realStorage, ctx, secCtx, map[string]any{
		objects.FieldKeyID:              atkID,
		objects.FieldKeyKind:            objects.KindAgentTask,
		objects.FieldKeyStatus:          statusInProgress,
		objects.FieldKeyEstimatedEffort: "1h",
	})
	applyDependencyRefEvent(ctx, logging.NewEventLogger(ctx), realStorage, DependencyRefEvent{
		ProjectRoot: t.TempDir(),
		EventID:     "test-atk-noreact-" + atkID,
		Version:     "2030-01-01T00:00:00.000000001Z",
		TargetID:    atkID,
		TriggerID:   bliID,
		TriggerKind: objects.KindBacklogItem,
		FromState:   statusInProgress,
		ToState:     statusComplete,
	})
	assertStatus(t, realStorage, ctx, secCtx, atkID, statusInProgress)
}

func TestApplyDependencyRefEvents_SelectiveCriteriaLock(t *testing.T) {
	// Mixed list: only shovel-ready CRITs hop. TRACK: BLI-REDACTED.
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	logger := logging.NewEventLogger(ctx)

	readyID := fixtureID(t, "CRIT-ready")
	lockedID := fixtureID(t, "CRIT-locked")
	doneID := fixtureID(t, "CRIT-done")
	bliID := fixtureID(t, "BLI")

	mustCreateCASVisible(t, realStorage, ctx, secCtx, map[string]any{
		objects.FieldKeyID: readyID, objects.FieldKeyKind: objects.KindCriteria,
		objects.FieldKeyStatus: objects.ObjectStatusAwaitingVerification, objects.FieldKeyCategory: "acceptance",
	})
	mustCreateCASVisible(t, realStorage, ctx, secCtx, map[string]any{
		objects.FieldKeyID: lockedID, objects.FieldKeyKind: objects.KindCriteria,
		objects.FieldKeyStatus: statusInProgress, objects.FieldKeyCategory: "acceptance",
	})
	mustCreateCASVisible(t, realStorage, ctx, secCtx, map[string]any{
		objects.FieldKeyID: doneID, objects.FieldKeyKind: objects.KindCriteria,
		objects.FieldKeyStatus: statusComplete, objects.FieldKeyCategory: "acceptance",
	})

	bli := map[string]any{
		objects.FieldKeyID:           bliID,
		objects.FieldKeyKind:         objects.KindBacklogItem,
		objects.FieldKeyStatus:       statusInProgress,
		objects.FieldKeyCriteriaRefs: []any{readyID, lockedID, doneID},
	}
	ApplyDependencyRefEvents(ctx, logger, realStorage, t.TempDir(), objects.KindBacklogItem, bliID, objects.ObjectStatusPlanned, statusInProgress, bli)

	assertStatus(t, realStorage, ctx, secCtx, readyID, statusInProgress)
	assertStatus(t, realStorage, ctx, secCtx, lockedID, statusInProgress)
	assertStatus(t, realStorage, ctx, secCtx, doneID, statusComplete)
}

func TestApplyDependencyRefEvents_NoCriteriaRefsIsNoop(t *testing.T) {
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())
	ctx := pkgctx.NewSystemContext()
	logger := logging.NewEventLogger(ctx)
	bliID := fixtureID(t, "BLI")
	bli := map[string]any{
		objects.FieldKeyID:     bliID,
		objects.FieldKeyKind:   objects.KindBacklogItem,
		objects.FieldKeyStatus: statusInProgress,
	}
	ApplyDependencyRefEvents(ctx, logger, realStorage, t.TempDir(), objects.KindBacklogItem, bliID, objects.ObjectStatusPlanned, statusInProgress, bli)
}

func TestDependencyPropagationSubscriber_HandleEvent_LocksCriteria(t *testing.T) {
	pool := testkit.PrepareGraphConnectionForTest(t)
	realStorage := storage.NewPoolAwareGraphStorage(pool, t.TempDir())
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	critID := fixtureID(t, "CRIT")
	bliID := fixtureID(t, "BLI")

	mustCreateCASVisible(t, realStorage, ctx, secCtx, map[string]any{
		objects.FieldKeyID:       critID,
		objects.FieldKeyKind:     objects.KindCriteria,
		objects.FieldKeyStatus:   objects.ObjectStatusAwaitingVerification,
		objects.FieldKeyCategory: "acceptance",
	})

	sub := NewDependencyPropagationSubscriber(func(projectRoot string) (storage.ObjectStorageProvider, bool) {
		if projectRoot != emptyValue {
			return realStorage, true
		}
		return nil, false
	})
	_ = sub.HandleEvent(&coordination.OperationalEvent{
		Type: coordination.EventTypeLifecycleDependencyRef,
		Metadata: map[string]any{
			"project_root":           "/proj",
			objects.FieldKeyTargetID: critID,
			"trigger_id":             bliID,
			"trigger_kind":           objects.KindBacklogItem,
			"from_state":             objects.ObjectStatusPlanned,
			"to_state":               statusInProgress,
		},
	})
	assertStatus(t, realStorage, ctx, secCtx, critID, statusInProgress)
}

func assertStatus(t *testing.T, store storage.ObjectStorageProvider, ctx context.Context, secCtx *pkgctx.SecurityContext, id, want string) {
	t.Helper()
	obj, err := store.Read(ctx, secCtx, id)
	if err != nil {
		t.Fatalf("read %s: %v", id, err)
	}
	if got, _ := obj[objects.FieldKeyStatus].(string); got != want {
		t.Errorf("%s status = %q, want %q", id, got, want)
	}
}
