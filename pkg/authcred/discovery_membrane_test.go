package authcred

import (
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestResolveDiscoveryLane(t *testing.T) {
	t.Parallel()
	if got := ResolveDiscoveryLane(nil); got != DiscoveryLaneFull {
		t.Fatalf("nil → full, got %s", got)
	}
	admin := &pkgctx.SecurityContext{Roles: []string{"admin"}}
	if got := ResolveDiscoveryLane(admin); got != DiscoveryLaneFull {
		t.Fatalf("admin → full, got %s", got)
	}
	planner := &pkgctx.SecurityContext{Permissions: []string{PermissionAgentOrchestrate}}
	if got := ResolveDiscoveryLane(planner); got != DiscoveryLanePlanner {
		t.Fatalf("orchestrate → planner, got %s", got)
	}
	doer := &pkgctx.SecurityContext{Permissions: []string{PermissionWriteCode, PermissionWriteAgentTask}}
	if got := ResolveDiscoveryLane(doer); got != DiscoveryLaneDoer {
		t.Fatalf("doer → doer, got %s", got)
	}
}

func TestFilterKindsForDiscovery(t *testing.T) {
	t.Parallel()
	all := []string{
		objects.KindAgentTask,
		objects.KindPriorityPlan,
		objects.KindGoal,
		"mystery_kind",
		objects.KindBacklogItem,
	}
	doer := &pkgctx.SecurityContext{Permissions: []string{PermissionWriteAgentTask}}
	got, lane := FilterKindsForDiscovery(doer, all, false)
	if lane != DiscoveryLaneDoer {
		t.Fatalf("lane=%s", lane)
	}
	if !slicesEqual(got, []string{objects.KindAgentTask, objects.KindBacklogItem}) {
		t.Fatalf("doer filtered=%v", got)
	}
	gotAll, _ := FilterKindsForDiscovery(doer, all, true)
	if !slicesEqual(gotAll, all) {
		t.Fatalf("--all-kinds should pass through: %v", gotAll)
	}
	planner := &pkgctx.SecurityContext{Permissions: []string{PermissionAgentOrchestrate}}
	gotP, laneP := FilterKindsForDiscovery(planner, all, false)
	if laneP != DiscoveryLanePlanner {
		t.Fatalf("lane=%s", laneP)
	}
	if !slicesEqual(gotP, []string{objects.KindAgentTask, objects.KindPriorityPlan, objects.KindGoal, objects.KindBacklogItem}) {
		t.Fatalf("planner filtered=%v", gotP)
	}
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
