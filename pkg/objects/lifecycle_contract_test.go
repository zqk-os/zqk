package objects

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Lifecycle YAML contract tests (table-driven from disk — no combinatorial matrix).

// strictAutoTriggerKinds require every auto-only non-system edge to declare on_dependent_status
// (DSL shockwave). Completion rollups are allowlisted until a dependents-all trigger exists.
var strictAutoTriggerKinds = map[string]struct{}{
	KindPriorityPlan: {},
}

// completionRollupAllowlist: auto-only edges without on_dependent_status that mean
// "all children done". PRI active|in_progress→complete are dual (manual+auto) so they
// are not auto-only and do not need this allowlist — see LIFECYCLE_STATUS_ROLES.md
// § First-class promote. TRACK: replace remaining auto-only rollups with
// on_all_dependents_status DSL (BLI-1785784867143912000-635942fb).
var completionRollupAllowlist = map[string]struct{}{}

type lifecycleYAMLDoc struct {
	ObjectType    string            `yaml:"object_type"`
	StatusMapping map[string]string `yaml:"status_mapping"`
	Statuses      []struct {
		Value         string   `yaml:"value"`
		Role          string   `yaml:"role"`
		Terminal      bool     `yaml:"terminal"`
		Archive       bool     `yaml:"archive"`
		WorkDone      bool     `yaml:"work_done"`
		Satisfied     bool     `yaml:"satisfied"`
		System        bool     `yaml:"system"`
		Preliminary   bool     `yaml:"preliminary"`
		Description   string   `yaml:"description"`
		Preconditions []string `yaml:"preconditions"`
	} `yaml:"statuses"`
	Transitions []struct {
		From             string   `yaml:"from"`
		To               string   `yaml:"to"`
		Manual           bool     `yaml:"manual"`
		Auto             bool     `yaml:"auto"`
		Description      string   `yaml:"description"`
		Catalyst         string   `yaml:"catalyst"`
		ClassVsSpecialty string   `yaml:"class_vs_specialty"`
		Preconditions    []string `yaml:"preconditions"`
		Postconditions   []string `yaml:"postconditions"`
		Shockwave        struct {
			Mode string `yaml:"mode"`
		} `yaml:"shockwave"`
		OnDependentStatus *struct {
			Kind string `yaml:"kind"`
		} `yaml:"on_dependent_status"`
	} `yaml:"transitions"`
}

func loadAllLifecycleDocs(t *testing.T) []lifecycleYAMLDoc {
	t.Helper()
	wd, err := fileutil.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, err := paths.ModuleRootFromPath(wd)
	if err != nil {
		t.Fatal(err)
	}
	dirs := []string{filepath.Join(root, paths.ProcessInternalLifecyclesDir)}
	for _, extra := range ExtraLifecycleRoots() {
		dirs = append(dirs, extra)
	}
	var out []lifecycleYAMLDoc
	for _, dir := range dirs {
		err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d == nil {
				return nil
			}
			if d.IsDir() || filepath.Ext(d.Name()) != ".yaml" {
				return nil
			}
			data, err := fileutil.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", d.Name(), err)
			}
			var doc lifecycleYAMLDoc
			if err := yaml.Unmarshal(data, &doc); err != nil {
				t.Fatalf("parse %s: %v", d.Name(), err)
			}
			out = append(out, doc)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func TestLifecycleContract_AutoOnlyNeverPromoteTarget(t *testing.T) {
	// Isolate each auto-only edge: alone, it must never appear as a promote target
	// (manual siblings to the same To are allowed elsewhere).
	for _, doc := range loadAllLifecycleDocs(t) {
		for _, tr := range doc.Transitions {
			if !tr.Auto || tr.Manual || tr.To == "" {
				continue
			}
			lc := &Lifecycle{Transitions: []Transition{{
				From: tr.From, To: tr.To, Auto: true, Manual: false,
			}}}
			froms := []string{tr.From}
			if tr.From == "*" {
				froms = nil
				for _, s := range doc.Statuses {
					if s.Value != "" {
						froms = append(froms, s.Value)
					}
				}
			}
			for _, from := range froms {
				if _, ok := PromoteTransitionTargets(lc, from)[tr.To]; ok {
					t.Errorf("%s: auto-only %s→%s must not be a promote target from %s",
						doc.ObjectType, tr.From, tr.To, from)
				}
			}
		}
	}
}

func TestLifecycleContract_StrictAutoEdgesHaveTrigger(t *testing.T) {
	for _, doc := range loadAllLifecycleDocs(t) {
		if _, strict := strictAutoTriggerKinds[doc.ObjectType]; !strict {
			continue
		}
		systemTo := map[string]bool{}
		for _, s := range doc.Statuses {
			if s.System || s.Value == ObjectStatusError {
				systemTo[s.Value] = true
			}
		}
		for _, tr := range doc.Transitions {
			if !tr.Auto || tr.Manual {
				continue
			}
			if systemTo[tr.To] {
				continue // system sink (*→error)
			}
			key := doc.ObjectType + ":" + tr.From + "->" + tr.To
			if _, ok := completionRollupAllowlist[key]; ok {
				continue
			}
			if tr.OnDependentStatus == nil || strings.TrimSpace(tr.OnDependentStatus.Kind) == "" {
				t.Errorf("%s: auto-only %s→%s needs on_dependent_status (or system sink / completion allowlist)",
					doc.ObjectType, tr.From, tr.To)
			}
		}
	}
}

func TestLifecycleContract_RoleInvariants(t *testing.T) {
	for _, doc := range loadAllLifecycleDocs(t) {
		for _, s := range doc.Statuses {
			if s.Value == "" {
				continue
			}
			if s.Role == "" {
				t.Errorf("%s/%s: missing role", doc.ObjectType, s.Value)
				continue
			}
			if (s.Terminal || s.Archive) && s.Role != LifecycleRoleTerminal {
				t.Errorf("%s/%s: terminal/archive flag requires role=terminal, got %q", doc.ObjectType, s.Value, s.Role)
			}
			if s.System && s.Role != LifecycleRoleHalted && s.Role != LifecycleRoleTerminal {
				t.Errorf("%s/%s: system flag expects role halted (or terminal), got %q", doc.ObjectType, s.Value, s.Role)
			}
			if s.Role == LifecycleRoleShovelReady && s.Preliminary {
				t.Errorf("%s/%s: shovel_ready must not be preliminary", doc.ObjectType, s.Value)
			}
			if s.Role == LifecycleRoleEnforced && s.Preliminary {
				t.Errorf("%s/%s: enforced must not be preliminary", doc.ObjectType, s.Value)
			}
			if strings.TrimSpace(s.Description) == "" {
				t.Errorf("%s/%s: missing description", doc.ObjectType, s.Value)
			}
		}
	}
}

func TestLifecycleContract_PolicyAndRoleActiveAreEnforced(t *testing.T) {
	// TRACK: BLI-CEF-R26-POLICY-PRI-EXAM-001 — membrane live is not Gantt shovel_ready.
	want := map[string]struct{}{KindPolicy: {}, KindRole: {}}
	found := map[string]string{}
	for _, doc := range loadAllLifecycleDocs(t) {
		if _, ok := want[doc.ObjectType]; !ok {
			continue
		}
		for _, s := range doc.Statuses {
			if s.Value == ObjectStatusActive {
				found[doc.ObjectType] = s.Role
			}
		}
	}
	for kind := range want {
		if found[kind] != LifecycleRoleEnforced {
			t.Errorf("%s status active: role %q, want enforced", kind, found[kind])
		}
	}
}

func TestLifecycleContract_PriorityPlanInProgressEquivActiveOrderZero(t *testing.T) {
	// Ranking only: execution_locked still outranks shovel_ready. This is not a status collapse.
	if PlanActiveOrderPenalty(KindPriorityPlan, map[string]any{FieldKeyStatus: ObjectStatusInProgress}) != 0 {
		t.Fatal("in_progress must have active_order penalty 0")
	}
	if PlanWhatsNextStatusBonus(KindPriorityPlan, ObjectStatusInProgress) <= PlanWhatsNextStatusBonus(KindPriorityPlan, ObjectStatusActive) {
		t.Fatal("execution_locked bonus must beat shovel_ready")
	}
}

func TestLifecycleContract_PriorityPlanExecutionCheckValve(t *testing.T) {
	// TRACK: BLI-1785439369431933000-f0cccd6c — in_progress is a check valve.
	forbidden := map[string]struct{}{
		ObjectStatusGrooming: {}, ObjectStatusActive: {},
		"planning": {}, "prioritizing": {},
	}
	allowed := map[string]struct{}{
		ObjectStatusPaused: {}, ObjectStatusBlocked: {},
		ObjectStatusComplete: {}, ObjectStatusCancelled: {},
	}
	var doc lifecycleYAMLDoc
	for _, d := range loadAllLifecycleDocs(t) {
		if d.ObjectType == KindPriorityPlan {
			doc = d
			break
		}
	}
	if doc.ObjectType == "" {
		t.Fatal("missing priority_plan lifecycle")
	}
	if mapped, ok := doc.StatusMapping["in_progress"]; ok && mapped != "in_progress" {
		t.Fatal("status_mapping must not collapse in_progress onto another sibling status")
	}
	for _, tr := range doc.Transitions {
		if tr.From != ObjectStatusInProgress {
			continue
		}
		if _, bad := forbidden[tr.To]; bad {
			t.Errorf("check valve: in_progress → %s is a demotion; only pause/halt/complete/cancel", tr.To)
		}
		if _, ok := allowed[tr.To]; !ok {
			t.Errorf("check valve: in_progress → %s is not an allowed exit", tr.To)
		}
	}
}

func TestLifecycleContract_PriorityPlanHaltDoesNotResumeToShovelReady(t *testing.T) {
	// TRACK: BLI-1785439369431933000-f0cccd6c — halt is not a launder back to active.
	var doc lifecycleYAMLDoc
	for _, d := range loadAllLifecycleDocs(t) {
		if d.ObjectType == KindPriorityPlan {
			doc = d
			break
		}
	}
	if doc.ObjectType == "" {
		t.Fatal("missing priority_plan lifecycle")
	}
	roleOf := map[string]string{}
	for _, s := range doc.Statuses {
		roleOf[s.Value] = s.Role
	}
	halted := map[string]struct{}{}
	for value, role := range roleOf {
		if role == LifecycleRoleHalted {
			halted[value] = struct{}{}
		}
	}
	if len(halted) == 0 {
		t.Fatal("priority_plan must annotate paused/blocked as halted")
	}
	for _, tr := range doc.Transitions {
		if _, ok := halted[tr.From]; !ok {
			continue
		}
		if roleOf[tr.To] == LifecycleRoleShovelReady {
			t.Errorf("check valve launder: halted %s → shovel_ready %s", tr.From, tr.To)
		}
		if tr.To == ObjectStatusActive {
			t.Errorf("halt resume must not target active: %s → %s", tr.From, tr.To)
		}
	}
}

func TestLifecycleContract_GanttPartnersHavePostconditions(t *testing.T) {
	// TRACK: BLI-CEF-R26-GANTT-PARTNERS-001 / CRIT-CEF-R26-GANTT-PARTNERS-001
	partners := map[string]struct{}{
		KindBacklogItem: {},
		KindAgentTask:   {},
		KindWorkstream:  {},
		KindRoadmap:     {},
		KindMilestone:   {},
		KindGoal:        {},
		KindRequirement: {},
		KindCriteria:    {},
	}
	found := map[string]int{}
	for _, doc := range loadAllLifecycleDocs(t) {
		if _, ok := partners[doc.ObjectType]; !ok {
			continue
		}
		n := 0
		for _, tr := range doc.Transitions {
			edgeN := 0
			for _, p := range tr.Postconditions {
				if strings.TrimSpace(p) != "" {
					edgeN++
					n++
				}
			}
			if edgeN == 0 {
				t.Errorf("%s: %s→%s has no postconditions", doc.ObjectType, tr.From, tr.To)
			}
		}
		found[doc.ObjectType] = n
	}
	for kind := range partners {
		if found[kind] == 0 {
			t.Errorf("%s: missing lifecycle or postconditions==0", kind)
		}
	}
}

func TestLifecycleContract_WorkstreamHaltResumesToShovelReady(t *testing.T) {
	// Lane pause is not a PRI check valve: paused → active (shovel_ready) is the resume.
	// TRACK: BLI-CEF-R26-GANTT-PARTNERS-001
	var doc lifecycleYAMLDoc
	for _, d := range loadAllLifecycleDocs(t) {
		if d.ObjectType == KindWorkstream {
			doc = d
			break
		}
	}
	if doc.ObjectType == "" {
		t.Fatal("missing workstream lifecycle")
	}
	var resume bool
	for _, tr := range doc.Transitions {
		if tr.From == ObjectStatusPaused && tr.To == ObjectStatusActive {
			resume = true
		}
	}
	if !resume {
		t.Fatal("workstream must keep paused→active (lane resume to shovel_ready)")
	}
}

func TestLifecycleContract_GoalBlockedResumesToActive(t *testing.T) {
	// Live program target: blocked → active. Do not copy PRI halted↛shovel_ready.
	// TRACK: BLI-CEF-R26-GANTT-PARTNERS-001
	var doc lifecycleYAMLDoc
	for _, d := range loadAllLifecycleDocs(t) {
		if d.ObjectType == KindGoal {
			doc = d
			break
		}
	}
	if doc.ObjectType == "" {
		t.Fatal("missing goal lifecycle")
	}
	var resume bool
	for _, tr := range doc.Transitions {
		if tr.From == ObjectStatusBlocked && tr.To == ObjectStatusActive {
			resume = true
		}
	}
	if !resume {
		t.Fatal("goal must keep blocked→active (commitment resume, not column unseal)")
	}
}

func TestLifecycleContract_WorkDoneAndSatisfiedFlags(t *testing.T) {
	wantWorkDone := map[string]string{
		KindBacklogItem:        ObjectStatusComplete,
		KindMilestone:          ObjectStatusComplete,
		KindAgentTask:          ObjectStatusImplemented,
		KindTechnicalDebt:      ObjectStatusResolved,
		KindPriorityPlan:       ObjectStatusComplete,
		KindWorkstream:         ObjectStatusComplete,
		KindRoadmap:            ObjectStatusComplete,
		KindGoal:               ObjectStatusComplete,
		KindStrategicPlan:      ObjectStatusComplete,
		KindRequirement:        ObjectStatusComplete,
		KindTestCase:           ObjectStatusComplete,
		KindConvergenceSession: "completed",
	}
	docs := map[string]lifecycleYAMLDoc{}
	for _, doc := range loadAllLifecycleDocs(t) {
		docs[doc.ObjectType] = doc
	}
	for kind, status := range wantWorkDone {
		doc, ok := docs[kind]
		if !ok {
			t.Errorf("missing lifecycle for %s", kind)
			continue
		}
		found := false
		for _, s := range doc.Statuses {
			if s.WorkDone && s.Archive {
				t.Errorf("%s/%s: work_done must not be set on archive", doc.ObjectType, s.Value)
			}
			if s.Value == status {
				found = true
				if !s.WorkDone {
					t.Errorf("%s/%s: expected work_done: true", kind, status)
				}
			}
		}
		if !found {
			t.Errorf("%s: status %s not in lifecycle", kind, status)
		}
	}
	crit, ok := docs[KindCriteria]
	if !ok {
		t.Fatal("missing criteria lifecycle")
	}
	for _, s := range crit.Statuses {
		switch s.Value {
		case ObjectStatusValidated, ObjectStatusComplete:
			if !s.Satisfied {
				t.Errorf("criteria/%s: expected satisfied: true", s.Value)
			}
			if s.WorkDone {
				t.Errorf("criteria/%s: must not be work_done", s.Value)
			}
		default:
			if s.Satisfied {
				t.Errorf("criteria/%s: unexpected satisfied", s.Value)
			}
		}
	}
}

// TestLifecycleContract_WorkflowActiveIsNotTerminal pins the activate hop.
// draft→active is the happy path; marking active terminal makes promote skip it
// (archive/system/non-progress). TRACK: BLI context 80e22512 — this contract
// was claimed 2026-08-04 and later regressed when lifecycle YAML regenerated.
func TestLifecycleContract_WorkflowActiveIsNotTerminal(t *testing.T) {
	found := false
	for _, doc := range loadAllLifecycleDocs(t) {
		if doc.ObjectType != KindWorkflow {
			continue
		}
		for _, s := range doc.Statuses {
			if s.Value != ObjectStatusActive {
				continue
			}
			found = true
			if s.Terminal {
				t.Errorf("workflow/active must not be terminal: draft→active is the activate hop")
			}
			if s.Role != LifecycleRoleShovelReady {
				t.Errorf("workflow/active role = %q, want %s", s.Role, LifecycleRoleShovelReady)
			}
		}
	}
	if !found {
		t.Fatal("workflow lifecycle missing active status")
	}
}

func TestLifecycleContract_WorkDoneDoesNotHoldTypedActualEffort(t *testing.T) {
	const typedActual = "actual_effort is set"
	for _, doc := range loadAllLifecycleDocs(t) {
		for _, s := range doc.Statuses {
			for _, p := range s.Preconditions {
				if strings.TrimSpace(p) == typedActual {
					t.Errorf("%s/%s: %q is membrane-autofilled; do not gate status on a typed actual", doc.ObjectType, s.Value, typedActual)
				}
			}
		}
		for _, tr := range doc.Transitions {
			for _, p := range tr.Preconditions {
				if strings.TrimSpace(p) == typedActual {
					t.Errorf("%s %s→%s: %q is membrane-autofilled; do not gate the hop on a typed actual", doc.ObjectType, tr.From, tr.To, typedActual)
				}
			}
		}
	}
}

func TestLifecycleContract_GanttPartnersHaveLifecycleExam(t *testing.T) {
	partners := map[string]struct{}{
		KindBacklogItem: {},
		KindAgentTask:   {},
		KindWorkstream:  {},
		KindRoadmap:     {},
		KindMilestone:   {},
		KindGoal:        {},
		KindRequirement: {},
		KindCriteria:    {},
	}
	found := map[string]int{}
	for _, doc := range loadAllLifecycleDocs(t) {
		if _, ok := partners[doc.ObjectType]; !ok {
			continue
		}
		n := 0
		for _, tr := range doc.Transitions {
			n++
			if strings.TrimSpace(tr.Catalyst) == "" {
				t.Errorf("%s: %s→%s missing catalyst", doc.ObjectType, tr.From, tr.To)
			}
			if tr.Shockwave.Mode == "" {
				t.Errorf("%s: %s→%s missing shockwave mode", doc.ObjectType, tr.From, tr.To)
			}
			if strings.TrimSpace(tr.ClassVsSpecialty) == "" {
				t.Errorf("%s: %s→%s missing class_vs_specialty", doc.ObjectType, tr.From, tr.To)
			}
		}
		found[doc.ObjectType] = n
	}
	for kind := range partners {
		if found[kind] == 0 {
			t.Errorf("%s: missing lifecycle or transitions==0", kind)
		}
	}
}
