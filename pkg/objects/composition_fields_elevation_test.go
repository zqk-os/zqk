package objects

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsCompositionFieldAllowed_VariousKinds(t *testing.T) {
	t.Parallel()
	// PriorityPlan specific
	assert.True(t, IsCompositionFieldAllowed(KindPriorityPlan, "commit_hashes", nil))
	assert.True(t, IsCompositionFieldAllowed(KindPriorityPlan, "commit_refs", nil))
	assert.True(t, IsCompositionFieldAllowed(KindPriorityPlan, "notes", nil))

	// TechnicalDebt specific
	assert.True(t, IsCompositionFieldAllowed(KindTechnicalDebt, "debt_type", nil))
	assert.True(t, IsCompositionFieldAllowed(KindTechnicalDebt, "impact_assessment", nil))
	assert.True(t, IsCompositionFieldAllowed(KindTechnicalDebt, "remediation", nil))
	assert.True(t, IsCompositionFieldAllowed(KindTechnicalDebt, "goal_refs", nil))

	// Policy specific
	assert.True(t, IsCompositionFieldAllowed(KindPolicy, "enforcement_level", nil))
	assert.True(t, IsCompositionFieldAllowed(KindPolicy, "custom_field_1", nil))
	assert.True(t, IsCompositionFieldAllowed(KindPolicy, "custom_field_2", nil))

	// AgentTask specific
	assert.True(t, IsCompositionFieldAllowed(KindAgentTask, "agent_heartbeat", nil))
	assert.True(t, IsCompositionFieldAllowed(KindAgentTask, "agent_pid", nil))
	assert.True(t, IsCompositionFieldAllowed(KindAgentTask, "branch_name", nil))
	assert.True(t, IsCompositionFieldAllowed(KindAgentTask, "task_steps", nil))

	// TestCase and Requirement specific
	assert.True(t, IsCompositionFieldAllowed(KindTestCase, "verification_hash", nil))
	assert.True(t, IsCompositionFieldAllowed(KindTestCase, "verified_artifact_refs", nil))
	assert.True(t, IsCompositionFieldAllowed(KindRequirement, "verification_hash", nil))

	// ZqkSession specific
	assert.True(t, IsCompositionFieldAllowed(KindZqkSession, FieldKeyAgreementMode, nil))
	assert.True(t, IsCompositionFieldAllowed(KindZqkSession, FieldKeyConsumerKernelRef, nil))
	assert.True(t, IsCompositionFieldAllowed(KindZqkSession, FieldKeyMaxUnits, nil))
	assert.True(t, IsCompositionFieldAllowed(KindZqkSession, FieldKeyRevokedAt, nil))
}

func TestSpecHasTrait(t *testing.T) {
	t.Parallel()
	assert.False(t, SpecHasTrait(nil, "any_trait"))

	spec := &Spec{
		ResolvedTraits: []string{"resolved_trait_a"},
		Traits:         []string{"declared_trait_b"},
	}
	assert.True(t, SpecHasTrait(spec, "resolved_trait_a"))
	assert.True(t, SpecHasTrait(spec, "declared_trait_b"))
	assert.False(t, SpecHasTrait(spec, "missing_trait"))
}
