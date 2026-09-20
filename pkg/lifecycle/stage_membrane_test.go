package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

type memMembraneStore struct {
	objs        map[string]map[string]any
	updateErr   map[string]error
	updateCalls []string
}

func (m *memMembraneStore) Read(_ context.Context, _ *pkgctx.SecurityContext, id string) (map[string]any, error) {
	obj, ok := m.objs[id]
	if !ok {
		return nil, fmt.Errorf("not found: %s", id)
	}
	return obj, nil
}

func (m *memMembraneStore) Update(_ context.Context, _ *pkgctx.SecurityContext, id string, updates map[string]any) error {
	if err := m.updateErr[id]; err != nil {
		return err
	}
	obj, ok := m.objs[id]
	if !ok {
		return fmt.Errorf("not found: %s", id)
	}
	if st, ok := updates[objects.FieldKeyStatus].(string); ok {
		obj[objects.FieldKeyStatus] = st
	}
	m.updateCalls = append(m.updateCalls, id)
	return nil
}

type staticLifecycles map[string]*objects.Lifecycle

func (s staticLifecycles) LoadLifecycle(kind string) (*objects.Lifecycle, error) {
	lc, ok := s[kind]
	if !ok {
		return nil, fmt.Errorf("no lifecycle for %s", kind)
	}
	return lc, nil
}

func archiveStarLifecycle() *objects.Lifecycle {
	return &objects.Lifecycle{
		Transitions: []objects.Transition{{From: "*", To: objects.ObjectStatusArchived, Manual: true}},
	}
}

func storyLifecycles() staticLifecycles {
	star := archiveStarLifecycle()
	return staticLifecycles{
		kindCriteria:     star,
		kindBacklogItem:  star,
		kindRequirement:  star,
		kindPriorityPlan: star,
	}
}

func TestTransitionAllowed_WildcardAndExact(t *testing.T) {
	t.Parallel()
	life := &objects.Lifecycle{
		Transitions: []objects.Transition{
			{From: "exploring", To: "deferred", Manual: true},
			{From: "*", To: "archived", Manual: true},
		},
	}
	if !TransitionAllowed(life, "exploring", "deferred") {
		t.Fatal("expected exploring→deferred")
	}
	if !TransitionAllowed(life, "rejected", "archived") {
		t.Fatal("expected *→archived from rejected")
	}
	if TransitionAllowed(life, "rejected", "deferred") {
		t.Fatal("rejected→deferred must be false without an edge")
	}
	if TransitionAllowed(life, "exploring", "planned") {
		t.Fatal("missing edge must be false")
	}
}

func TestTransitionAllowed_UnknownFromRepairsToPark(t *testing.T) {
	t.Parallel()
	life := &objects.Lifecycle{
		Statuses: []objects.Status{
			{Value: "identified"},
			{Value: objects.ObjectStatusDeferred},
			{Value: objects.ObjectStatusArchived},
		},
		Transitions: []objects.Transition{
			{From: "identified", To: objects.ObjectStatusDeferred, Manual: true},
			{From: "*", To: objects.ObjectStatusArchived, Manual: true},
		},
	}
	if !TransitionAllowed(life, "accepted", objects.ObjectStatusDeferred) {
		t.Fatal("unknown from must repair-park to deferred")
	}
	if !TransitionAllowed(life, "accepted", objects.ObjectStatusArchived) {
		t.Fatal("unknown from still matches *→archived")
	}
	if TransitionAllowed(life, "accepted", "identified") {
		t.Fatal("unknown from must not hop to non-park statuses")
	}
	if !TransitionAllowed(life, "identified", objects.ObjectStatusArchived) {
		t.Fatal("known from still matches *→archived")
	}
	if TransitionAllowed(life, "identified", "planned") {
		t.Fatal("known from without edge must stay false")
	}
}

func TestPlanStageMembraneHop_ParksCRITBLIREQCluster(t *testing.T) {
	t.Parallel()
	store := &memMembraneStore{objs: map[string]map[string]any{
		"CRIT-1": {
			objects.FieldKeyID:     "CRIT-1",
			objects.FieldKeyKind:   kindCriteria,
			objects.FieldKeyStatus: objects.ObjectStatusAwaitingVerification,
		},
		"BLI-1": {
			objects.FieldKeyID:           "BLI-1",
			objects.FieldKeyKind:         kindBacklogItem,
			objects.FieldKeyStatus:       objects.ObjectStatusComplete,
			objects.FieldKeyCriteriaRefs: []any{"CRIT-1"},
		},
		"REQ-1": {
			objects.FieldKeyID:              "REQ-1",
			objects.FieldKeyKind:            kindRequirement,
			objects.FieldKeyStatus:          objects.ObjectStatusActive,
			objects.FieldKeyCriteriaRefs:    []any{"CRIT-1"},
			objects.FieldKeyBacklogItemRefs: []any{"BLI-1"},
		},
		"GOAL-1": {
			objects.FieldKeyID:     "GOAL-1",
			objects.FieldKeyKind:   objects.KindGoal,
			objects.FieldKeyStatus: objects.ObjectStatusActive,
		},
	}}
	deps := func(id string) []string {
		switch id {
		case "CRIT-1":
			return []string{"BLI-1", "REQ-1"}
		case "BLI-1":
			return []string{"REQ-1", "GOAL-1"}
		case "GOAL-1":
			return []string{"BLI-1"}
		default:
			return nil
		}
	}
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	plan, err := PlanStageMembraneHop(ctx, sec, store, storyLifecycles(), deps, []string{"CRIT-1"}, objects.ObjectStatusArchived)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	got := map[string]bool{}
	for _, m := range plan.Members {
		got[m.ID] = true
		if m.Kind == objects.KindGoal {
			t.Fatalf("goal %s must stay lineage, not cluster", m.ID)
		}
	}
	for _, id := range []string{"CRIT-1", "BLI-1", "REQ-1"} {
		if !got[id] {
			t.Fatalf("missing cluster member %s", id)
		}
	}
	if got["GOAL-1"] {
		t.Fatal("GOAL-1 must not be in the archive cluster")
	}
	if len(plan.LineageParents) == 0 {
		t.Fatal("expected lineage parent rollup for GOAL-1")
	}
	if plan.LineageParents[0].ID != "GOAL-1" {
		t.Fatalf("lineage parent = %s, want GOAL-1", plan.LineageParents[0].ID)
	}
	if plan.LineageParents[0].Status != "active" {
		t.Fatalf("goal status = %s, want active (trunk stays)", plan.LineageParents[0].Status)
	}
	if plan.Policy.Mode != objects.ShockwaveModeCluster {
		t.Fatalf("mode = %s, want cluster", plan.Policy.Mode)
	}
}

func TestPlanStageMembraneHop_RefusesArchiveBLIWhenPlanNotArchived(t *testing.T) {
	t.Parallel()
	store := &memMembraneStore{objs: map[string]map[string]any{
		"PRI-1": {
			objects.FieldKeyID:     "PRI-1",
			objects.FieldKeyKind:   kindPriorityPlan,
			objects.FieldKeyStatus: objects.ObjectStatusComplete,
		},
		"BLI-1": {
			objects.FieldKeyID:              "BLI-1",
			objects.FieldKeyKind:            kindBacklogItem,
			objects.FieldKeyStatus:          objects.ObjectStatusComplete,
			objects.FieldKeyPriorityPlanRef: "PRI-1",
		},
	}}
	_, err := PlanStageMembraneHop(context.Background(), pkgctx.NewSystemSecurityContext(), store, storyLifecycles(), func(string) []string { return nil }, []string{"BLI-1"}, objects.ObjectStatusArchived)
	var blocked *MembraneHopBlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("want MembraneHopBlockedError, got %v", err)
	}
	if blocked.BlockingID != "BLI-1" {
		t.Fatalf("blocking = %s, want BLI-1", blocked.BlockingID)
	}
}

func TestPlanStageMembraneHop_AllowsArchiveBLIWhenPlanArchived(t *testing.T) {
	t.Parallel()
	store := &memMembraneStore{objs: map[string]map[string]any{
		"PRI-1": {
			objects.FieldKeyID:     "PRI-1",
			objects.FieldKeyKind:   kindPriorityPlan,
			objects.FieldKeyStatus: objects.ObjectStatusArchived,
		},
		"BLI-1": {
			objects.FieldKeyID:              "BLI-1",
			objects.FieldKeyKind:            kindBacklogItem,
			objects.FieldKeyStatus:          objects.ObjectStatusComplete,
			objects.FieldKeyPriorityPlanRef: "PRI-1",
		},
	}}
	plan, err := PlanStageMembraneHop(context.Background(), pkgctx.NewSystemSecurityContext(), store, storyLifecycles(), func(string) []string { return nil }, []string{"BLI-1"}, objects.ObjectStatusArchived)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	got := false
	for _, m := range plan.Members {
		if m.ID == "BLI-1" {
			got = true
		}
		if m.ID == "PRI-1" {
			t.Fatal("archived plan must stay lineage, not hop with the BLI seed")
		}
	}
	if !got {
		t.Fatal("missing BLI-1 member")
	}
}

func TestPlanStageMembraneHop_BlockedHopFreezesTree(t *testing.T) {
	t.Parallel()
	store := &memMembraneStore{objs: map[string]map[string]any{
		"CRIT-1": {
			objects.FieldKeyID:     "CRIT-1",
			objects.FieldKeyKind:   kindCriteria,
			objects.FieldKeyStatus: objects.ObjectStatusAwaitingVerification,
		},
		"BLI-1": {
			objects.FieldKeyID:           "BLI-1",
			objects.FieldKeyKind:         kindBacklogItem,
			objects.FieldKeyStatus:       objects.ObjectStatusComplete,
			objects.FieldKeyCriteriaRefs: []any{"CRIT-1"},
		},
	}}
	lifecycles := staticLifecycles{
		kindCriteria:    archiveStarLifecycle(),
		kindBacklogItem: {Transitions: []objects.Transition{{From: "exploring", To: "archived"}}},
	}
	deps := func(id string) []string {
		if id == "CRIT-1" {
			return []string{"BLI-1"}
		}
		return nil
	}
	_, err := PlanStageMembraneHop(context.Background(), pkgctx.NewSystemSecurityContext(), store, lifecycles, deps, []string{"CRIT-1"}, objects.ObjectStatusArchived)
	var blocked *MembraneHopBlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("want MembraneHopBlockedError, got %v", err)
	}
	if blocked.BlockingID != "BLI-1" {
		t.Fatalf("blocking id = %s, want BLI-1", blocked.BlockingID)
	}
	if store.objs["CRIT-1"][objects.FieldKeyStatus] != "awaiting_verification" {
		t.Fatal("seed must stay on prior membrane when the tree is blocked")
	}
	if store.objs["BLI-1"][objects.FieldKeyStatus] != "complete" {
		t.Fatal("blocked member must not change")
	}
}

func TestPlanStageMembraneHop_MissingOutboundRefBlocks(t *testing.T) {
	t.Parallel()
	store := &memMembraneStore{objs: map[string]map[string]any{
		"BLI-1": {
			objects.FieldKeyID:           "BLI-1",
			objects.FieldKeyKind:         kindBacklogItem,
			objects.FieldKeyStatus:       objects.ObjectStatusComplete,
			objects.FieldKeyCriteriaRefs: []any{"CRIT-MISSING"},
		},
	}}
	_, err := PlanStageMembraneHop(context.Background(), pkgctx.NewSystemSecurityContext(), store, storyLifecycles(), nil, []string{"BLI-1"}, objects.ObjectStatusArchived)
	var blocked *MembraneHopBlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("want MembraneHopBlockedError, got %v", err)
	}
	if blocked.BlockingID != "CRIT-MISSING" {
		t.Fatalf("blocking id = %s, want CRIT-MISSING", blocked.BlockingID)
	}
}

func TestPlanStageMembraneHop_PriorityPlanIncludesChildBLIs(t *testing.T) {
	t.Parallel()
	store := &memMembraneStore{objs: map[string]map[string]any{
		"PRI-1": {
			objects.FieldKeyID:     "PRI-1",
			objects.FieldKeyKind:   kindPriorityPlan,
			objects.FieldKeyStatus: objects.ObjectStatusActive,
		},
		"BLI-1": {
			objects.FieldKeyID:              "BLI-1",
			objects.FieldKeyKind:            kindBacklogItem,
			objects.FieldKeyStatus:          objects.ObjectStatusComplete,
			objects.FieldKeyPriorityPlanRef: "PRI-1",
			objects.FieldKeyCriteriaRefs:    []any{"CRIT-1"},
		},
		"CRIT-1": {
			objects.FieldKeyID:     "CRIT-1",
			objects.FieldKeyKind:   kindCriteria,
			objects.FieldKeyStatus: objects.ObjectStatusArchived,
		},
	}}
	deps := func(id string) []string {
		if id == "PRI-1" {
			return []string{"BLI-1"}
		}
		if id == "CRIT-1" {
			return []string{"BLI-1"}
		}
		return nil
	}
	plan, err := PlanStageMembraneHop(context.Background(), pkgctx.NewSystemSecurityContext(), store, storyLifecycles(), deps, []string{"PRI-1"}, objects.ObjectStatusArchived)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	got := map[string]MembraneMember{}
	for _, m := range plan.Members {
		got[m.ID] = m
	}
	if _, ok := got["PRI-1"]; !ok {
		t.Fatal("missing PRI-1")
	}
	if _, ok := got["BLI-1"]; !ok {
		t.Fatal("missing child BLI-1")
	}
	if !got["CRIT-1"].Skip {
		t.Fatal("already-archived CRIT should skip")
	}
}

func TestApplyStageMembraneHop_AllOrNamedImpediment(t *testing.T) {
	t.Parallel()
	store := &memMembraneStore{objs: map[string]map[string]any{
		"CRIT-1": {
			objects.FieldKeyID:     "CRIT-1",
			objects.FieldKeyKind:   kindCriteria,
			objects.FieldKeyStatus: objects.ObjectStatusAwaitingVerification,
		},
		"BLI-1": {
			objects.FieldKeyID:           "BLI-1",
			objects.FieldKeyKind:         kindBacklogItem,
			objects.FieldKeyStatus:       objects.ObjectStatusComplete,
			objects.FieldKeyCriteriaRefs: []any{"CRIT-1"},
		},
	}}
	deps := func(id string) []string {
		if id == "CRIT-1" {
			return []string{"BLI-1"}
		}
		return nil
	}
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	plan, err := PlanStageMembraneHop(ctx, sec, store, storyLifecycles(), deps, []string{"CRIT-1"}, objects.ObjectStatusArchived)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if err := ApplyStageMembraneHop(ctx, sec, store, deps, plan); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if store.objs["CRIT-1"][objects.FieldKeyStatus] != objects.ObjectStatusArchived {
		t.Fatal("CRIT-1 not archived")
	}
	if store.objs["BLI-1"][objects.FieldKeyStatus] != objects.ObjectStatusArchived {
		t.Fatal("BLI-1 not archived")
	}
}

func TestPlanStageMembraneHop_PruneGoalCascadesBranchNotVision(t *testing.T) {
	t.Parallel()
	store := &memMembraneStore{objs: map[string]map[string]any{
		"GOAL-1": {
			objects.FieldKeyID:        "GOAL-1",
			objects.FieldKeyKind:      kindGoal,
			objects.FieldKeyStatus:    objects.ObjectStatusActive,
			objects.FieldKeyVisionRef: "VIS-1",
		},
		"BLI-1": {
			objects.FieldKeyID:           "BLI-1",
			objects.FieldKeyKind:         kindBacklogItem,
			objects.FieldKeyStatus:       objects.ObjectStatusComplete,
			objects.FieldKeyGoalRefs:     []any{"GOAL-1"},
			objects.FieldKeyCriteriaRefs: []any{"CRIT-1"},
		},
		"CRIT-1": {
			objects.FieldKeyID:     "CRIT-1",
			objects.FieldKeyKind:   kindCriteria,
			objects.FieldKeyStatus: objects.ObjectStatusAwaitingVerification,
		},
		"VIS-1": {
			objects.FieldKeyID:     "VIS-1",
			objects.FieldKeyKind:   kindVision,
			objects.FieldKeyStatus: objects.ObjectStatusActive,
		},
	}}
	lc := storyLifecycles()
	lc[kindGoal] = &objects.Lifecycle{
		Statuses: []objects.Status{{
			Value:     objects.ObjectStatusArchived,
			Shockwave: objects.ShockwavePolicy{Mode: objects.ShockwaveModePrune, Fail: objects.ShockwaveFailClosed},
		}},
		Transitions: []objects.Transition{{From: "*", To: objects.ObjectStatusArchived, Manual: true}},
	}
	lc[kindVision] = archiveStarLifecycle()
	deps := func(id string) []string {
		switch id {
		case "GOAL-1":
			// Child→parent: BLI points at GOAL. Vision is above GOAL via goal.vision_ref,
			// not a reverse dependent of the goal.
			return []string{"BLI-1"}
		case "CRIT-1":
			return []string{"BLI-1"}
		default:
			return nil
		}
	}
	plan, err := PlanStageMembraneHop(context.Background(), pkgctx.NewSystemSecurityContext(), store, lc, deps, []string{"GOAL-1"}, objects.ObjectStatusArchived)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if plan.Policy.Mode != objects.ShockwaveModePrune {
		t.Fatalf("mode = %s, want prune", plan.Policy.Mode)
	}
	got := map[string]bool{}
	for _, m := range plan.Members {
		got[m.ID] = true
	}
	if !got["GOAL-1"] || !got["BLI-1"] || !got["CRIT-1"] {
		t.Fatalf("prune members = %v, want GOAL+BLI+CRIT", got)
	}
	if got["VIS-1"] {
		t.Fatal("vision is the trunk; prune must not archive it")
	}
	foundVision := false
	for _, p := range plan.LineageParents {
		if p.ID == "VIS-1" {
			foundVision = true
			if p.Status != "active" {
				t.Fatalf("vision rollup status = %s, want active", p.Status)
			}
		}
	}
	if !foundVision {
		t.Fatal("prune of a goal must leave the vision as lineage_parents rollup (trunk stays)")
	}
}

func TestApplyStageMembraneHop_LineageRollupShowsCollapsedBranch(t *testing.T) {
	t.Parallel()
	store := &memMembraneStore{objs: map[string]map[string]any{
		"CRIT-1": {
			objects.FieldKeyID:     "CRIT-1",
			objects.FieldKeyKind:   kindCriteria,
			objects.FieldKeyStatus: objects.ObjectStatusAwaitingVerification,
		},
		"BLI-1": {
			objects.FieldKeyID:           "BLI-1",
			objects.FieldKeyKind:         kindBacklogItem,
			objects.FieldKeyStatus:       objects.ObjectStatusComplete,
			objects.FieldKeyCriteriaRefs: []any{"CRIT-1"},
			objects.FieldKeyGoalRefs:     []any{"GOAL-1"},
		},
		"GOAL-1": {
			objects.FieldKeyID:     "GOAL-1",
			objects.FieldKeyKind:   kindGoal,
			objects.FieldKeyStatus: objects.ObjectStatusActive,
		},
	}}
	deps := func(id string) []string {
		switch id {
		case "CRIT-1":
			return []string{"BLI-1"}
		case "BLI-1":
			return []string{"GOAL-1"}
		case "GOAL-1":
			return []string{"BLI-1"}
		default:
			return nil
		}
	}
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()
	plan, err := PlanStageMembraneHop(ctx, sec, store, storyLifecycles(), deps, []string{"CRIT-1"}, objects.ObjectStatusArchived)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if err := ApplyStageMembraneHop(ctx, sec, store, deps, plan); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if store.objs["GOAL-1"][objects.FieldKeyStatus] != "active" {
		t.Fatal("goal must stay active")
	}
	if len(plan.LineageParents) == 0 {
		t.Fatal("missing lineage rollup")
	}
	parent := plan.LineageParents[0]
	if parent.ArchivedDependents < 1 {
		t.Fatalf("archived dependents = %d, want ≥1 after branch collapse", parent.ArchivedDependents)
	}
	if parent.Status != "active" {
		t.Fatalf("rollup status = %s, want active", parent.Status)
	}
}

func TestApplyStageMembraneHop_WriteFailureNamesImpediment(t *testing.T) {
	t.Parallel()
	store := &memMembraneStore{
		objs: map[string]map[string]any{
			"CRIT-1": {
				objects.FieldKeyID:     "CRIT-1",
				objects.FieldKeyKind:   kindCriteria,
				objects.FieldKeyStatus: objects.ObjectStatusAwaitingVerification,
			},
		},
		updateErr: map[string]error{"CRIT-1": errors.New("cas write denied")},
	}
	plan, err := PlanStageMembraneHop(context.Background(), pkgctx.NewSystemSecurityContext(), store, storyLifecycles(), nil, []string{"CRIT-1"}, objects.ObjectStatusArchived)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	err = ApplyStageMembraneHop(context.Background(), pkgctx.NewSystemSecurityContext(), store, nil, plan)
	var blocked *MembraneHopBlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("want MembraneHopBlockedError, got %v", err)
	}
	if blocked.BlockingID != "CRIT-1" {
		t.Fatalf("blocking id = %s", blocked.BlockingID)
	}
}

func sharedArchiveLifecycle(fail string, exclusive ...string) *objects.Lifecycle {
	return &objects.Lifecycle{
		Statuses: []objects.Status{{
			Value: objects.ObjectStatusArchived,
			Shockwave: objects.ShockwavePolicy{
				Mode:           objects.ShockwaveModeShared,
				Fail:           fail,
				ExclusiveKinds: exclusive,
			},
		}},
		Transitions: []objects.Transition{{From: "*", To: objects.ObjectStatusArchived, Manual: true}},
	}
}

func TestPlanStageMembraneHop_PrunePRIStopsAtWorkstream(t *testing.T) {
	t.Parallel()
	store := &memMembraneStore{objs: map[string]map[string]any{
		"PRI-1": {
			objects.FieldKeyID:            "PRI-1",
			objects.FieldKeyKind:          kindPriorityPlan,
			objects.FieldKeyStatus:        objects.ObjectStatusActive,
			objects.FieldKeyWorkstreamRef: "WS-1",
		},
		"BLI-1": {
			objects.FieldKeyID:              "BLI-1",
			objects.FieldKeyKind:            kindBacklogItem,
			objects.FieldKeyStatus:          objects.ObjectStatusComplete,
			objects.FieldKeyPriorityPlanRef: "PRI-1",
			objects.FieldKeyWorkstreamRefs:  []any{"WS-1"},
		},
		"WS-1": {
			objects.FieldKeyID:     "WS-1",
			objects.FieldKeyKind:   kindWorkstream,
			objects.FieldKeyStatus: objects.ObjectStatusActive,
		},
	}}
	lc := storyLifecycles()
	lc[kindPriorityPlan] = &objects.Lifecycle{
		Statuses: []objects.Status{{
			Value:     objects.ObjectStatusArchived,
			Shockwave: objects.ShockwavePolicy{Mode: objects.ShockwaveModePrune, Fail: objects.ShockwaveFailClosed},
		}},
		Transitions: []objects.Transition{{From: "*", To: objects.ObjectStatusArchived, Manual: true}},
	}
	lc[kindWorkstream] = archiveStarLifecycle()
	deps := func(id string) []string {
		switch id {
		case "PRI-1":
			return []string{"BLI-1"}
		case "WS-1":
			return []string{"PRI-1", "BLI-1"}
		default:
			return nil
		}
	}
	plan, err := PlanStageMembraneHop(context.Background(), pkgctx.NewSystemSecurityContext(), store, lc, deps, []string{"PRI-1"}, objects.ObjectStatusArchived)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	got := map[string]bool{}
	for _, m := range plan.Members {
		got[m.ID] = true
	}
	if !got["PRI-1"] || !got["BLI-1"] {
		t.Fatalf("prune members = %v, want PRI+BLI", got)
	}
	if got["WS-1"] {
		t.Fatal("workstream is a shared lane; prune must not archive sibling-plan infrastructure")
	}
	foundWS := false
	for _, p := range plan.LineageParents {
		if p.ID == "WS-1" {
			foundWS = true
			if p.Status != objects.ObjectStatusActive {
				t.Fatalf("workstream rollup status = %s, want active", p.Status)
			}
		}
	}
	if !foundWS {
		t.Fatal("prune of a plan must leave the workstream as lineage_parents rollup")
	}
}

func TestPlanStageMembraneHop_SharedWorkstreamBlocksLivePRI(t *testing.T) {
	t.Parallel()
	store := &memMembraneStore{objs: map[string]map[string]any{
		"WS-1": {
			objects.FieldKeyID:     "WS-1",
			objects.FieldKeyKind:   kindWorkstream,
			objects.FieldKeyStatus: objects.ObjectStatusActive,
		},
		"PRI-1": {
			objects.FieldKeyID:            "PRI-1",
			objects.FieldKeyKind:          kindPriorityPlan,
			objects.FieldKeyStatus:        objects.ObjectStatusActive,
			objects.FieldKeyWorkstreamRef: "WS-1",
		},
		"BLI-1": {
			objects.FieldKeyID:             "BLI-1",
			objects.FieldKeyKind:           kindBacklogItem,
			objects.FieldKeyStatus:         objects.ObjectStatusInProgress,
			objects.FieldKeyWorkstreamRefs: []any{"WS-1"},
		},
	}}
	lc := storyLifecycles()
	lc[kindWorkstream] = sharedArchiveLifecycle(objects.ShockwaveFailClosed, objects.KindWorkstreamTransition)
	deps := func(id string) []string {
		if id == "WS-1" {
			return []string{"PRI-1", "BLI-1"}
		}
		return nil
	}
	_, err := PlanStageMembraneHop(context.Background(), pkgctx.NewSystemSecurityContext(), store, lc, deps, []string{"WS-1"}, objects.ObjectStatusArchived)
	var blocked *MembraneHopBlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("want MembraneHopBlockedError, got %v", err)
	}
	if blocked.BlockingID != "WS-1" {
		t.Fatalf("blocking id = %s", blocked.BlockingID)
	}
}

func TestPlanStageMembraneHop_SharedWorkstreamArchivesLaneNotBLIs(t *testing.T) {
	t.Parallel()
	store := &memMembraneStore{objs: map[string]map[string]any{
		"WS-1": {
			objects.FieldKeyID:     "WS-1",
			objects.FieldKeyKind:   kindWorkstream,
			objects.FieldKeyStatus: objects.ObjectStatusActive,
		},
		"PRI-1": {
			objects.FieldKeyID:            "PRI-1",
			objects.FieldKeyKind:          kindPriorityPlan,
			objects.FieldKeyStatus:        objects.ObjectStatusComplete,
			objects.FieldKeyWorkstreamRef: "WS-1",
		},
		"BLI-1": {
			objects.FieldKeyID:             "BLI-1",
			objects.FieldKeyKind:           kindBacklogItem,
			objects.FieldKeyStatus:         objects.ObjectStatusComplete,
			objects.FieldKeyWorkstreamRefs: []any{"WS-1"},
		},
	}}
	lc := storyLifecycles()
	lc[kindWorkstream] = sharedArchiveLifecycle(objects.ShockwaveFailClosed, objects.KindWorkstreamTransition)
	deps := func(id string) []string {
		if id == "WS-1" {
			return []string{"PRI-1", "BLI-1"}
		}
		return nil
	}
	plan, err := PlanStageMembraneHop(context.Background(), pkgctx.NewSystemSecurityContext(), store, lc, deps, []string{"WS-1"}, objects.ObjectStatusArchived)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if plan.Policy.Mode != objects.ShockwaveModeShared {
		t.Fatalf("mode = %s, want shared", plan.Policy.Mode)
	}
	got := map[string]bool{}
	for _, m := range plan.Members {
		got[m.ID] = true
	}
	if !got["WS-1"] {
		t.Fatal("workstream seed must hop")
	}
	if got["BLI-1"] || got["PRI-1"] {
		t.Fatalf("shared lane must not cascade sibling plans or BLIs: %v", got)
	}
}

func TestPlanStageMembraneHop_SharedRoadmapFailOpenLeavesWorkstream(t *testing.T) {
	t.Parallel()
	store := &memMembraneStore{objs: map[string]map[string]any{
		"RDM-1": {
			objects.FieldKeyID:             "RDM-1",
			objects.FieldKeyKind:           kindRoadmap,
			objects.FieldKeyStatus:         objects.ObjectStatusActive,
			objects.FieldKeyWorkstreamRefs: []any{"WS-1"},
		},
		"WS-1": {
			objects.FieldKeyID:     "WS-1",
			objects.FieldKeyKind:   kindWorkstream,
			objects.FieldKeyStatus: objects.ObjectStatusActive,
		},
	}}
	lc := storyLifecycles()
	lc[kindRoadmap] = sharedArchiveLifecycle(objects.ShockwaveFailOpen)
	lc[kindWorkstream] = archiveStarLifecycle()
	deps := func(id string) []string {
		if id == "WS-1" {
			return []string{"RDM-1"}
		}
		return nil
	}
	plan, err := PlanStageMembraneHop(context.Background(), pkgctx.NewSystemSecurityContext(), store, lc, deps, []string{"RDM-1"}, objects.ObjectStatusArchived)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	got := map[string]bool{}
	for _, m := range plan.Members {
		got[m.ID] = true
	}
	if !got["RDM-1"] {
		t.Fatal("roadmap seed must hop")
	}
	if got["WS-1"] {
		t.Fatal("roadmap is a view; archiving it must not prune workstream lanes")
	}
}
