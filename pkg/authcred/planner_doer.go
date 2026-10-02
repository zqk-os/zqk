// Package authcred — planner vs doer lane helpers (POL-AGENT-PLANNER-DOER-001).
// TRACK: follow-up in kernel backlog
package authcred

import (
	"slices"
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
)

// Permission strings used by planner/doer role matrix.
const (
	PermissionAgentOrchestrate = "agent:orchestrate"
	PermissionWriteCode        = "write:code"
	PermissionWriteAgentTask   = "write:agent_task"
)

// StrategicKernelKinds are doer-denied write targets (POL-AGENT-PLANNER-DOER-001).
var StrategicKernelKinds = []string{
	objects.KindPriorityPlan,
	objects.KindGoal,
	objects.KindMilestone,
	objects.KindRequirement,
	objects.KindPolicy,
	objects.KindTeam,
	"team_configuration",
	objects.KindStrategicPlan,
	objects.KindCriteria,
}

// RoleMatches reports whether an assigned ACC role label matches a role object's
// id or role_id. Aliases are data on the role (RoleRecord.Aliases), not a code table.
func RoleMatches(assigned, roleObjID, roleID string) bool {
	return (RoleRecord{ID: roleObjID, RoleID: roleID}).Matches(assigned)
}

// ExpandAssignedRoles returns assigned labels plus matching role_id values from dir.
func ExpandAssignedRoles(assigned []string, dir SeatDirectory) []string {
	c := newStringCollector(len(assigned)*2, false)
	for _, a := range assigned {
		c.Add(a)
	}
	if dir == nil {
		return c.Result()
	}
	for _, role := range dir.Roles() {
		if role.MatchesAny(assigned) {
			c.Add(role.RoleID)
			c.Add(role.ID)
		}
	}
	return c.Result()
}

// HasExactPermission reports whether secCtx lists the permission (no wildcards).
func HasExactPermission(secCtx *pkgctx.SecurityContext, perm string) bool {
	if secCtx == nil {
		return false
	}
	return slices.Contains(secCtx.Permissions, perm)
}

// HasWriteCode is true when the seat may act as a code implementer.
func HasWriteCode(secCtx *pkgctx.SecurityContext) bool {
	if secCtx == nil {
		return false
	}
	if slices.Contains(secCtx.Roles, "admin") || secCtx.AccountID == pkgctx.SystemAccountID {
		return true
	}
	for _, p := range secCtx.Permissions {
		if p == PermissionWriteCode || p == "write:*" {
			return true
		}
	}
	return false
}

// MayOrchestratePeers is true for planner/CAP seats (agent:orchestrate or strategic writes).
func MayOrchestratePeers(secCtx *pkgctx.SecurityContext) bool {
	if secCtx == nil {
		return false
	}
	if secCtx.AccountID == pkgctx.SystemAccountID || slices.Contains(secCtx.Roles, "admin") {
		return true
	}
	for _, p := range secCtx.Permissions {
		if p == PermissionAgentOrchestrate || p == "write:*" {
			return true
		}
		if p == "write:priority_plan" || p == "write:agent_instruction" || p == "write:strategic_plan" {
			return true
		}
	}
	return false
}

// DenyOrchestrateIfDoer returns an error when a doer seat tries peer orchestration.
func DenyOrchestrateIfDoer(secCtx *pkgctx.SecurityContext) error {
	if MayOrchestratePeers(secCtx) {
		return nil
	}
	return errfmt.Errorf("permission denied: doer seats cannot orchestrate peers (POL-AGENT-PLANNER-DOER-001); need %s or planner strategic write permissions", PermissionAgentOrchestrate)
}

// DenyPeerSteerIfDoer denies directed feed steer for doer seats.
func DenyPeerSteerIfDoer(secCtx *pkgctx.SecurityContext, toAgentID string) error {
	if strings.TrimSpace(toAgentID) == "" {
		return nil
	}
	return DenyOrchestrateIfDoer(secCtx)
}

// IsStrategicKernelKind reports kinds doers must not administer.
func IsStrategicKernelKind(kind string) bool {
	return slices.Contains(StrategicKernelKinds, kind)
}

// PlannerLaneForbiddenPerms must not appear on canonical planner agent roles.
var PlannerLaneForbiddenPerms = []string{PermissionWriteCode}

// DoerLaneForbiddenPerms must not appear on canonical doer roles (swarm/coder).
var DoerLaneForbiddenPerms = []string{
	PermissionAgentOrchestrate,
	"write:priority_plan",
	"write:goal",
	"write:requirement",
	"write:policy",
	"write:team_configuration",
	"write:milestone",
	"write:*",
	pkgctx.PermissionDeleteObjectDraftPlane,
	pkgctx.PermissionDeleteCore,
	pkgctx.PermissionDeleteAll,
}

// ValidateDoerPermissions returns an error if perms include cross-lane privileges.
func ValidateDoerPermissions(perms []string) error {
	for _, bad := range DoerLaneForbiddenPerms {
		if slices.Contains(perms, bad) {
			return errfmt.Errorf("doer role must not include %s (POL-AGENT-PLANNER-DOER-001)", bad)
		}
	}
	return nil
}

// ValidatePlannerPermissions returns an error if perms include implementer code write.
func ValidatePlannerPermissions(perms []string) error {
	for _, bad := range PlannerLaneForbiddenPerms {
		if slices.Contains(perms, bad) {
			return errfmt.Errorf("planner role must not include %s (POL-AGENT-PLANNER-DOER-001)", bad)
		}
	}
	return nil
}
