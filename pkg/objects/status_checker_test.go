package objects

import (
	"testing"
)

func TestStatusChecker_IsTerminal(t *testing.T) {
	checker := GetGlobalStatusChecker()

	// Test known kinds
	if !checker.IsTerminal("backlog_item", "complete") {
		t.Error("Expected backlog_item 'complete' to be terminal")
	}
	if checker.IsTerminal("backlog_item", "in_progress") {
		t.Error("Expected backlog_item 'in_progress' not to be terminal")
	}

	// Test fallback for unknown kinds (resolves to base_object_lifecycle.yaml)
	if !checker.IsTerminal("unknown_kind", "implemented") {
		t.Error("Expected unknown_kind 'implemented' to be terminal via base lifecycle fallback")
	}
	if checker.IsTerminal("unknown_kind", "complete") {
		t.Error("Expected unknown_kind 'complete' not to be terminal since base lifecycle uses 'implemented'")
	}
	if !checker.IsTerminal("backlog_item", "completed") {
		t.Error("Expected backlog_item 'completed' to resolve to terminal via alias mapping")
	}
}

func TestStatusChecker_IsPreliminary(t *testing.T) {
	checker := GetGlobalStatusChecker()

	// Test known kinds
	if !checker.IsPreliminary("backlog_item", "exploring") {
		t.Error("Expected backlog_item 'exploring' to be preliminary")
	}
	if checker.IsPreliminary("backlog_item", "in_progress") {
		t.Error("Expected backlog_item 'in_progress' not to be preliminary")
	}
	// Parked-but-materialized: reachable only from validated/planned via object park, so
	// treating it as preliminary would pull a live CAS object back onto the draft plane.
	if checker.IsPreliminary("backlog_item", "deferred") {
		t.Error("Expected backlog_item 'deferred' not to be preliminary")
	}
	if checker.IsPreliminary("backlog_item", "roadmap") {
		t.Error("Expected backlog_item 'roadmap' not to be preliminary")
	}

	// Test fallback for unknown kinds (resolves to base_object_lifecycle.yaml)
	if !checker.IsPreliminary("unknown_kind", "proposed") {
		t.Error("Expected unknown_kind 'proposed' to be preliminary via base lifecycle fallback")
	}

	// Born-complete origin (preliminary:false) must not park on the draft plane.
	if checker.IsPreliminary("audit_aggregation_metric", "completed") {
		t.Error("Expected audit_aggregation_metric 'completed' not to be preliminary")
	}
	if checker.IsPreliminary("glossary_term", "active") {
		t.Error("Expected glossary_term 'active' not to be preliminary")
	}
}

func TestStatusChecker_IsActive(t *testing.T) {
	checker := GetGlobalStatusChecker()

	// Test known kinds
	if !checker.IsActive("backlog_item", "in_progress") {
		t.Error("Expected backlog_item 'in_progress' to be active")
	}
	if checker.IsActive("backlog_item", "exploring") {
		t.Error("Expected backlog_item 'exploring' not to be active")
	}
	if checker.IsActive("backlog_item", "complete") {
		t.Error("Expected backlog_item 'complete' not to be active")
	}

	// Test fallback for unknown kinds (resolves to base_object_lifecycle.yaml)
	if !checker.IsActive("unknown_kind", "approved") {
		t.Error("Expected unknown_kind 'approved' to be active via base lifecycle fallback")
	}
}

func TestStatusChecker_Role(t *testing.T) {
	checker := GetGlobalStatusChecker()
	cases := []struct {
		kind, status, want string
	}{
		{"backlog_item", "planned", LifecycleRoleShovelReady},
		{"backlog_item", "in_progress", LifecycleRoleExecutionLocked},
		{"backlog_item", "exploring", LifecycleRoleRealign},
		{"backlog_item", "error", LifecycleRoleHalted},
		{"backlog_item", "complete", LifecycleRoleTerminal},
		{"priority_plan", "active", LifecycleRoleShovelReady},
		{"priority_plan", "in_progress", LifecycleRoleExecutionLocked},
		{"priority_plan", "grooming", LifecycleRoleGrooming},
		{"priority_plan", "paused", LifecycleRoleHalted},
	}
	for _, tc := range cases {
		got := checker.Role(tc.kind, tc.status)
		if got != tc.want {
			t.Errorf("Role(%q, %q) = %q, want %q", tc.kind, tc.status, got, tc.want)
		}
	}
	if !RoleAllowedOnExecutionFacingPlan(LifecycleRoleHalted) {
		t.Error("halted must be allowed on execution-facing plans (membership)")
	}
	if RoleReadyOrLaterForLock(LifecycleRoleHalted) {
		t.Error("halted must not satisfy airtight lock")
	}
	if !RolePlanRequiresReadyChildren(LifecycleRoleShovelReady) || !RolePlanRequiresReadyChildren(LifecycleRoleExecutionLocked) {
		t.Error("shovel_ready and execution_locked plans require ready children")
	}
}

func TestStatusChecker_IsWorkDone(t *testing.T) {
	checker := GetGlobalStatusChecker()
	if !checker.IsWorkDone(KindBacklogItem, ObjectStatusComplete) {
		t.Error("backlog_item complete is work_done")
	}
	if !checker.IsWorkDone(KindTechnicalDebt, ObjectStatusResolved) {
		t.Error("technical_debt resolved is work_done")
	}
	if !checker.IsWorkDone(KindAgentTask, ObjectStatusImplemented) {
		t.Error("agent_task implemented is work_done")
	}
	if checker.IsWorkDone(KindBacklogItem, ObjectStatusArchived) {
		t.Error("archive is not work_done")
	}
	if checker.IsWorkDone(KindTechnicalDebt, ObjectStatusArchived) {
		t.Error("TDE archive is not work_done")
	}
	if checker.IsWorkDone(KindCriteria, ObjectStatusValidated) {
		t.Error("criteria validated is satisfied, not work_done")
	}
}

func TestStatusChecker_IsSatisfied(t *testing.T) {
	checker := GetGlobalStatusChecker()
	if !checker.IsSatisfied(KindCriteria, ObjectStatusValidated) {
		t.Error("criteria validated is satisfied")
	}
	if !checker.IsSatisfied(KindCriteria, ObjectStatusValidated) {
		t.Error("criteria validated is satisfied")
	}
	if checker.IsSatisfied(KindCriteria, ObjectStatusInProgress) {
		t.Error("criteria in_progress is not satisfied")
	}
	if checker.IsSatisfied(KindBacklogItem, ObjectStatusComplete) {
		t.Error("BLI complete is work_done, not satisfiable")
	}
}

func TestStatusChecker_IsArchiveAndIsSystem(t *testing.T) {
	checker := GetGlobalStatusChecker()
	if !checker.IsArchive(KindBacklogItem, ObjectStatusArchived) {
		t.Error("expected backlog_item archived to be archive")
	}
	if checker.IsArchive(KindBacklogItem, ObjectStatusInProgress) {
		t.Error("expected backlog_item in_progress not to be archive")
	}
	if checker.IsArchive("nonexistent_kind", "some_status") {
		t.Error("expected false for nonexistent kind/status")
	}

	if !checker.IsSystem(KindBacklogItem, ObjectStatusError) {
		t.Error("expected backlog_item error to be system")
	}
	if checker.IsSystem(KindBacklogItem, ObjectStatusInProgress) {
		t.Error("expected backlog_item in_progress not to be system")
	}
	if checker.IsSystem("nonexistent_kind", "some_status") {
		t.Error("expected false for nonexistent kind/status")
	}
}
