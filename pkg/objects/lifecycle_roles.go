package objects

// Cross-kind lifecycle status roles (lifecycle YAML status.role).
const (
	LifecycleRoleGrooming        = "grooming"
	LifecycleRoleShovelReady     = "shovel_ready"
	LifecycleRoleExecutionLocked = "execution_locked"
	LifecycleRoleEnforced        = "enforced"
	LifecycleRoleRealign         = "realign"
	LifecycleRoleHalted          = "halted"
	LifecycleRoleTerminal        = "terminal"
)

// KnownLifecycleRoles is the closed class-role set (schema enum + YAML contract tests).
func KnownLifecycleRoles() []string {
	return []string{
		LifecycleRoleGrooming,
		LifecycleRoleShovelReady,
		LifecycleRoleExecutionLocked,
		LifecycleRoleEnforced,
		LifecycleRoleRealign,
		LifecycleRoleHalted,
		LifecycleRoleTerminal,
	}
}

// IsKnownLifecycleRole reports whether role is one of KnownLifecycleRoles.
func IsKnownLifecycleRole(role string) bool {
	switch role {
	case LifecycleRoleGrooming, LifecycleRoleShovelReady, LifecycleRoleExecutionLocked,
		LifecycleRoleEnforced, LifecycleRoleRealign, LifecycleRoleHalted, LifecycleRoleTerminal:
		return true
	default:
		return false
	}
}

// roleProgressRank orders the on-ladder roles from preliminary to terminal. This is the sequence
// every kind's lifecycle walks regardless of the labels it spells that sequence with: a plan calls
// shovel_ready "active" and a backlog item calls it "planned", but both sit at the same rung.
//
// realign, halted, and enforced are deliberately absent. realign steps back for rework,
// halted is a stop, and enforced is membrane-live (policy/role active) — not a Gantt column.
// Asking for their rank is a question with no answer, and RoleProgressRank says so rather
// than returning an ordinal that would make "is this further along?" silently wrong.
var roleProgressRank = map[string]int{
	LifecycleRoleGrooming:        0,
	LifecycleRoleShovelReady:     1,
	LifecycleRoleExecutionLocked: 2,
	LifecycleRoleTerminal:        3,
}

// RoleProgressRank returns the role's position on the preliminary-to-terminal ladder. ok is false for
// off-ladder roles (realign, halted, enforced) and unknown roles, which callers must handle rather
// than treating as rank 0.
func RoleProgressRank(role string) (rank int, ok bool) {
	r, ok := roleProgressRank[role]
	return r, ok
}

// RoleAllowedOnExecutionFacingPlan reports whether a child with this role may link to an
// active/in_progress (execution-facing) priority plan. Halted is allowed so recovery is not deadlocked.
func RoleAllowedOnExecutionFacingPlan(role string) bool {
	switch role {
	case LifecycleRoleShovelReady, LifecycleRoleExecutionLocked, LifecycleRoleTerminal, LifecycleRoleHalted:
		return true
	default:
		return false
	}
}

// RoleReadyOrLaterForLock reports whether a child role satisfies airtight lock
// (active/paused → in_progress). Halted does NOT count — repair first.
func RoleReadyOrLaterForLock(role string) bool {
	switch role {
	case LifecycleRoleShovelReady, LifecycleRoleExecutionLocked, LifecycleRoleTerminal:
		return true
	default:
		return false
	}
}

// RolePlanRequiresReadyChildren is true for execution-facing plan roles.
func RolePlanRequiresReadyChildren(role string) bool {
	switch role {
	case LifecycleRoleShovelReady, LifecycleRoleExecutionLocked:
		return true
	default:
		return false
	}
}
