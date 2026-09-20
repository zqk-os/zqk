package validation

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

// TestRefStatusRules_statusesExistInReferencedLifecycle checks the matrix against the lifecycle
// plane. A row naming a status the referenced kind does not have is the failure mode a table
// invites: it never matches, so the barrier silently refuses everything or admits everything
// depending on which list the typo landed in, and no test of the evaluator would notice.
func TestRefStatusRules_statusesExistInReferencedLifecycle(t *testing.T) {
	t.Parallel()
	loader := objects.NewLifecycleLoader(filepath.Join("..", "..", paths.ProcessInternalLifecyclesDir))
	if len(refStatusRules) == 0 {
		t.Fatal("no ref-status rules; this check would be vacuous")
	}
	for _, rule := range refStatusRules {
		lc, err := loader.LoadLifecycle(rule.RefKind)
		if err != nil {
			t.Errorf("rule %q references kind %q with no loadable lifecycle: %v", rule.Precondition, rule.RefKind, err)
			continue
		}
		valid := map[string]bool{}
		for _, st := range lc.Statuses {
			valid[st.Value] = true
		}
		if len(rule.Require) == 0 {
			t.Errorf("rule %q has no Require statuses, so it can never be satisfied", rule.Precondition)
		}
		for _, group := range [][]string{rule.Require, rule.Ignore} {
			for _, status := range group {
				if valid[status] || valid[objects.ApplyAliasesForStatus(status, valid)] {
					continue
				}
				t.Errorf("rule %q: %q is not a status of kind %q (has: %v)",
					rule.Precondition, status, rule.RefKind, sortedKeys(valid))
			}
		}
	}
}

// TestEvalRefStatus_isGenericOverAnyRule exercises the evaluator with a row that is not in the
// shipped matrix, so adding a barrier is demonstrably adding data rather than adding code.
func TestEvalRefStatus_isGenericOverAnyRule(t *testing.T) {
	t.Parallel()
	gv := NewGoValidator()
	rule := refStatusRule{
		Precondition: "synthetic: milestone_refs must be complete",
		Field:        objects.FieldKeyMilestoneRefs,
		RefKind:      objects.KindMilestone,
		Require:      []string{objects.ObjectStatusComplete},
	}
	subject := func(refs ...string) map[string]any {
		anyRefs := make([]any, 0, len(refs))
		for _, r := range refs {
			anyRefs = append(anyRefs, r)
		}
		return map[string]any{objects.FieldKeyMilestoneRefs: anyRefs}
	}
	statuses := func(m map[string]string) *ValidationOptions {
		return &ValidationOptions{ObjectStatusLookup: func(id string) (string, error) {
			st, ok := m[id]
			if !ok {
				return "", errors.New("not found")
			}
			return st, nil
		}}
	}

	if !gv.evalRefStatus(rule, subject("MIL-1", "MIL-2"), statuses(map[string]string{"MIL-1": "complete", "MIL-2": "complete"})) {
		t.Error("all refs complete must satisfy")
	}
	if gv.evalRefStatus(rule, subject("MIL-1", "MIL-2"), statuses(map[string]string{"MIL-1": "complete", "MIL-2": "in_progress"})) {
		t.Error("one incomplete ref must refuse: the constraint is over all refs, not any")
	}
	if gv.evalRefStatus(rule, subject(), statuses(nil)) {
		t.Error("no refs must refuse")
	}
	if gv.evalRefStatus(rule, subject("MIL-GONE"), statuses(nil)) {
		t.Error("unresolvable ref must refuse rather than count as satisfied")
	}
	// Archived lineage is opt-in per row, so this rule must not inherit the criteria allowance.
	if gv.evalRefStatus(rule, subject("MIL-1"), statuses(map[string]string{"MIL-1": objects.ObjectStatusArchived})) {
		t.Error("archived ref must refuse unless the row opts into ArchivedLineageSatisfies")
	}
}

// TestLookupRefStatusRule_toleratesLifecycleParentheticals pins the binding between a lifecycle
// phrase and its row. The shipped lifecycles append explanatory suffixes such as
// "(execution-facing)", and an exact-match lookup would leave those barriers unbound.
func TestShippedBacklogCompleteHopsBindCriteriaRow(t *testing.T) {
	t.Parallel()
	loader := objects.NewLifecycleLoader(filepath.Join("..", "..", paths.ProcessInternalLifecyclesDir))
	lc, err := loader.LoadLifecycle(objects.KindBacklogItem)
	if err != nil || lc == nil {
		t.Fatalf("load backlog_item lifecycle: %v", err)
	}
	wantHops := map[string]bool{
		"in_progress->complete": false,
		"planned->complete":     false,
	}
	for _, tr := range lc.Transitions {
		key := tr.From + "->" + tr.To
		if _, want := wantHops[key]; !want {
			continue
		}
		bound := false
		for _, p := range tr.Preconditions {
			if rule, ok := lookupRefStatusRule(strings.ToLower(p)); ok && rule.Field == objects.FieldKeyCriteriaRefs {
				bound = true
				break
			}
		}
		if !bound {
			t.Errorf("shipped %s did not bind the criteria ref-status row; preconds=%v", key, tr.Preconditions)
		}
		wantHops[key] = true
	}
	for key, seen := range wantHops {
		if !seen {
			t.Errorf("shipped backlog_item missing hop %s", key)
		}
	}
}

func TestLookupRefStatusRule_toleratesLifecycleParentheticals(t *testing.T) {
	t.Parallel()
	got, ok := lookupRefStatusRule("priority_plan_ref target must be in active or in_progress status (execution-facing)")
	if !ok || got.Field != objects.FieldKeyPriorityPlanRef {
		t.Fatalf("suffixed lifecycle phrase did not bind to the plan row: ok=%v field=%q", ok, got.Field)
	}
	if _, ok := lookupRefStatusRule("owner confirms the plan looks fine"); ok {
		t.Error("unrelated prose must not bind to a row")
	}

	rule, ok := lookupRefStatusRule("linked priority_plan is archived when priority_plan_ref is set")
	if !ok || rule.Field != objects.FieldKeyPriorityPlanRef {
		t.Fatalf("archived plan phrase did not bind to priority_plan_ref: ok=%v rule=%+v", ok, rule)
	}
	if !rule.AllowEmptyRef {
		t.Errorf("expected AllowEmptyRef to be true for optional priority_plan_ref archived rule")
	}
}

func TestShippedBacklogArchivedHopsBindPriorityPlanRow(t *testing.T) {
	t.Parallel()
	loader := objects.NewLifecycleLoader(filepath.Join("..", "..", paths.ProcessInternalLifecyclesDir))
	lc, err := loader.LoadLifecycle(objects.KindBacklogItem)
	if err != nil || lc == nil {
		t.Fatalf("load backlog_item lifecycle: %v", err)
	}
	found := false
	for _, tr := range lc.Transitions {
		if tr.To != objects.ObjectStatusArchived {
			continue
		}
		for _, p := range tr.Preconditions {
			if rule, ok := lookupRefStatusRule(strings.ToLower(p)); ok && rule.Field == objects.FieldKeyPriorityPlanRef {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("shipped backlog_item transition to archived did not bind the priority_plan_ref row")
	}
}

func TestEvalRefStatus_PriorityPlanArchivedWhenSet(t *testing.T) {
	t.Parallel()
	gv := NewGoValidator()
	rule, ok := lookupRefStatusRule(strings.ToLower(PrecondPriorityPlanArchivedWhenSet))
	if !ok {
		t.Fatalf("could not look up rule for %q", PrecondPriorityPlanArchivedWhenSet)
	}

	subject := func(planRef string) map[string]any {
		m := map[string]any{}
		if planRef != "" {
			m[objects.FieldKeyPriorityPlanRef] = planRef
		}
		return m
	}
	statuses := func(m map[string]string) *ValidationOptions {
		return &ValidationOptions{ObjectStatusLookup: func(id string) (string, error) {
			st, ok := m[id]
			if !ok {
				return "", errors.New("not found")
			}
			return st, nil
		}}
	}

	// 1. When priority_plan_ref is unset, passes vacuously
	if !gv.evalRefStatus(rule, subject(""), statuses(nil)) {
		t.Error("unset priority_plan_ref must satisfy vacuously")
	}

	// 2. When priority_plan_ref points to an archived plan, passes
	if !gv.evalRefStatus(rule, subject("PRI-ARCHIVED"), statuses(map[string]string{"PRI-ARCHIVED": objects.ObjectStatusArchived})) {
		t.Error("archived priority_plan must satisfy")
	}

	// 3. When priority_plan_ref points to an active or in_progress plan, fails
	if gv.evalRefStatus(rule, subject("PRI-ACTIVE"), statuses(map[string]string{"PRI-ACTIVE": objects.ObjectStatusActive})) {
		t.Error("active priority_plan must fail")
	}
	if gv.evalRefStatus(rule, subject("PRI-IN-PROGRESS"), statuses(map[string]string{"PRI-IN-PROGRESS": objects.ObjectStatusInProgress})) {
		t.Error("in_progress priority_plan must fail")
	}

	// 4. When priority_plan_ref cannot be resolved, fails (fail-closed)
	if gv.evalRefStatus(rule, subject("PRI-GHOST"), statuses(nil)) {
		t.Error("unresolvable priority_plan must fail")
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
