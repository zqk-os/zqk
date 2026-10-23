package objects

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKnownLifecycleRoles_Members(t *testing.T) {
	t.Parallel()
	roles := KnownLifecycleRoles()
	assert.Len(t, roles, 7)
	assert.Contains(t, roles, LifecycleRoleGrooming)
	assert.Contains(t, roles, LifecycleRoleShovelReady)
	assert.Contains(t, roles, LifecycleRoleExecutionLocked)
	assert.Contains(t, roles, LifecycleRoleEnforced)
	assert.Contains(t, roles, LifecycleRoleRealign)
	assert.Contains(t, roles, LifecycleRoleHalted)
	assert.Contains(t, roles, LifecycleRoleTerminal)
}

func TestIsKnownLifecycleRole(t *testing.T) {
	t.Parallel()
	assert.True(t, IsKnownLifecycleRole(LifecycleRoleGrooming))
	assert.True(t, IsKnownLifecycleRole(LifecycleRoleShovelReady))
	assert.True(t, IsKnownLifecycleRole(LifecycleRoleExecutionLocked))
	assert.True(t, IsKnownLifecycleRole(LifecycleRoleEnforced))
	assert.True(t, IsKnownLifecycleRole(LifecycleRoleRealign))
	assert.True(t, IsKnownLifecycleRole(LifecycleRoleHalted))
	assert.True(t, IsKnownLifecycleRole(LifecycleRoleTerminal))

	assert.False(t, IsKnownLifecycleRole(""))
	assert.False(t, IsKnownLifecycleRole("invalid_role"))
	assert.False(t, IsKnownLifecycleRole("active"))
}

func TestRoleProgressRank(t *testing.T) {
	t.Parallel()
	rank, ok := RoleProgressRank(LifecycleRoleGrooming)
	assert.True(t, ok)
	assert.Equal(t, 0, rank)

	rank, ok = RoleProgressRank(LifecycleRoleShovelReady)
	assert.True(t, ok)
	assert.Equal(t, 1, rank)

	rank, ok = RoleProgressRank(LifecycleRoleExecutionLocked)
	assert.True(t, ok)
	assert.Equal(t, 2, rank)

	rank, ok = RoleProgressRank(LifecycleRoleTerminal)
	assert.True(t, ok)
	assert.Equal(t, 3, rank)

	_, ok = RoleProgressRank(LifecycleRoleRealign)
	assert.False(t, ok)

	_, ok = RoleProgressRank(LifecycleRoleHalted)
	assert.False(t, ok)

	_, ok = RoleProgressRank(LifecycleRoleEnforced)
	assert.False(t, ok)

	_, ok = RoleProgressRank("unknown")
	assert.False(t, ok)
}

func TestRoleExecutionPlanConstraints(t *testing.T) {
	t.Parallel()
	// Allowed on execution facing plan: ShovelReady, ExecutionLocked, Terminal, Halted
	assert.True(t, RoleAllowedOnExecutionFacingPlan(LifecycleRoleShovelReady))
	assert.True(t, RoleAllowedOnExecutionFacingPlan(LifecycleRoleExecutionLocked))
	assert.True(t, RoleAllowedOnExecutionFacingPlan(LifecycleRoleTerminal))
	assert.True(t, RoleAllowedOnExecutionFacingPlan(LifecycleRoleHalted))
	assert.False(t, RoleAllowedOnExecutionFacingPlan(LifecycleRoleGrooming))
	assert.False(t, RoleAllowedOnExecutionFacingPlan(LifecycleRoleEnforced))
	assert.False(t, RoleAllowedOnExecutionFacingPlan(LifecycleRoleRealign))
	assert.False(t, RoleAllowedOnExecutionFacingPlan("unknown"))

	// Ready or later for lock: ShovelReady, ExecutionLocked, Terminal (NOT halted)
	assert.True(t, RoleReadyOrLaterForLock(LifecycleRoleShovelReady))
	assert.True(t, RoleReadyOrLaterForLock(LifecycleRoleExecutionLocked))
	assert.True(t, RoleReadyOrLaterForLock(LifecycleRoleTerminal))
	assert.False(t, RoleReadyOrLaterForLock(LifecycleRoleHalted))
	assert.False(t, RoleReadyOrLaterForLock(LifecycleRoleGrooming))

	// Plan requires ready children: ShovelReady, ExecutionLocked
	assert.True(t, RolePlanRequiresReadyChildren(LifecycleRoleShovelReady))
	assert.True(t, RolePlanRequiresReadyChildren(LifecycleRoleExecutionLocked))
	assert.False(t, RolePlanRequiresReadyChildren(LifecycleRoleTerminal))
	assert.False(t, RolePlanRequiresReadyChildren(LifecycleRoleHalted))
	assert.False(t, RolePlanRequiresReadyChildren(LifecycleRoleGrooming))
}
