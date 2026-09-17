// Package authcred — persona/RBAC-scoped object kind discovery (command membrane).
// TRACK: BLI-REDACTED — POL-AGENT-PLANNER-DOER-001 discoverability.
package authcred

import (
	"slices"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

// DiscoveryLane classifies how wide the default object kind catalog should be.
type DiscoveryLane string

const (
	DiscoveryLaneFull    DiscoveryLane = "full"    // admin / break-glass / system
	DiscoveryLanePlanner DiscoveryLane = "planner" // strategic + planning kinds
	DiscoveryLaneDoer    DiscoveryLane = "doer"    // assignment-relevant kinds
)

// DoerDiscoveryKinds is the default catalog for swarm/doer seats.
// Explicit `object <kind> …` still works for any registered kind; this only
// scopes kind enumeration (fields --list-kinds, bare list/count).
var DoerDiscoveryKinds = []string{
	objects.KindAgentTask,
	objects.KindAgentSkill,
	objects.KindAgentInstruction,
	objects.KindCriteria,
	objects.KindTestCase,
	objects.KindTechnicalDebt,
	objects.KindQuestion,
	objects.KindBacklogItem, // doers may read assignment context; writes gated elsewhere
}

// PlannerDiscoveryKinds is the default catalog for planner/orchestrator seats.
var PlannerDiscoveryKinds = []string{
	objects.KindPriorityPlan,
	objects.KindStrategicPlan,
	objects.KindGoal,
	objects.KindMilestone,
	objects.KindRequirement,
	objects.KindPolicy,
	objects.KindCriteria,
	objects.KindBacklogItem,
	objects.KindTeam,
	"team_configuration",
	objects.KindAgentTask,
	objects.KindAgentInstruction,
	objects.KindPersona,
	objects.KindRole,
	objects.KindAccount,
	objects.KindConvergenceSession,
	objects.KindWorkstream,
	objects.KindWorkflow,
	objects.KindQuestion,
	objects.KindAgentSkill,
}

// ResolveDiscoveryLane picks the default kind membrane for a security context.
// Admin / system / write:* / owner → full. Peer orchestration / strategic write → planner.
// Otherwise → doer.
func ResolveDiscoveryLane(secCtx *pkgctx.SecurityContext) DiscoveryLane {
	if secCtx == nil {
		return DiscoveryLaneFull
	}
	if secCtx.AccountID == pkgctx.SystemAccountID ||
		slices.Contains(secCtx.Roles, "admin") ||
		slices.Contains(secCtx.Roles, "owner") ||
		HasExactPermission(secCtx, "write:*") {
		return DiscoveryLaneFull
	}
	if MayOrchestratePeers(secCtx) {
		return DiscoveryLanePlanner
	}
	return DiscoveryLaneDoer
}

// KindAllowedInDiscovery reports whether kind is in the lane's default allow-set.
func KindAllowedInDiscovery(lane DiscoveryLane, kind string) bool {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return false
	}
	switch lane {
	case DiscoveryLaneFull:
		return true
	case DiscoveryLanePlanner:
		return slices.Contains(PlannerDiscoveryKinds, kind)
	case DiscoveryLaneDoer:
		return slices.Contains(DoerDiscoveryKinds, kind)
	default:
		return true
	}
}

// FilterKindsForDiscovery returns kinds visible under the seat membrane.
// When allKinds is true (break-glass --all-kinds) or lane is full, kinds is returned unchanged.
func FilterKindsForDiscovery(secCtx *pkgctx.SecurityContext, kinds []string, allKinds bool) (filtered []string, lane DiscoveryLane) {
	lane = ResolveDiscoveryLane(secCtx)
	if allKinds || lane == DiscoveryLaneFull {
		return kinds, lane
	}
	allow := DoerDiscoveryKinds
	if lane == DiscoveryLanePlanner {
		allow = PlannerDiscoveryKinds
	}
	set := make(map[string]struct{}, len(allow))
	for _, k := range allow {
		set[k] = struct{}{}
	}
	out := make([]string, 0, len(allow))
	for _, k := range kinds {
		if _, ok := set[k]; ok {
			out = append(out, k)
		}
	}
	return out, lane
}
