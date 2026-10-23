package objects

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFillShockwavePolicyDefaults(t *testing.T) {
	t.Parallel()
	emptyPolicy := ShockwavePolicy{}
	filled := FillShockwavePolicyDefaults(emptyPolicy)

	assert.Equal(t, ShockwaveFailClosed, filled.Fail)
	assert.NotEmpty(t, filled.LineageKinds)
	assert.NotEmpty(t, filled.ClusterKinds)
	assert.NotEmpty(t, filled.ClusterRefFields)
	assert.NotEmpty(t, filled.LineageRefFields)

	custom := ShockwavePolicy{
		Fail:             ShockwaveFailOpen,
		LineageKinds:     []string{"custom_trunk"},
		ClusterKinds:     []string{"custom_leaf"},
		ClusterRefFields: []string{"custom_ref"},
		LineageRefFields: []string{"custom_lineage_ref"},
	}
	filledCustom := FillShockwavePolicyDefaults(custom)
	assert.Equal(t, ShockwaveFailOpen, filledCustom.Fail)
	assert.Equal(t, []string{"custom_trunk"}, filledCustom.LineageKinds)
	assert.Equal(t, []string{"custom_leaf"}, filledCustom.ClusterKinds)
	assert.Equal(t, []string{"custom_ref"}, filledCustom.ClusterRefFields)
	assert.Equal(t, []string{"custom_lineage_ref"}, filledCustom.LineageRefFields)
}

func TestMergeShockwavePolicy_FullOverlay(t *testing.T) {
	t.Parallel()
	parent := DefaultArchiveShockwavePolicy()
	child := ShockwavePolicy{
		Mode:             ShockwaveModeShared,
		Fail:             ShockwaveFailOpen,
		LineageKinds:     []string{KindVision},
		ClusterKinds:     []string{KindCriteria},
		ClusterRefFields: []string{FieldKeyCriteriaRefs},
		LineageRefFields: []string{FieldKeyVisionRef},
		ExclusiveKinds:   []string{"edge_doc"},
	}
	merged := MergeShockwavePolicy(parent, child)
	assert.Equal(t, ShockwaveModeShared, merged.Mode)
	assert.Equal(t, ShockwaveFailOpen, merged.Fail)
	assert.Equal(t, []string{KindVision}, merged.LineageKinds)
	assert.Equal(t, []string{KindCriteria}, merged.ClusterKinds)
	assert.Equal(t, []string{FieldKeyCriteriaRefs}, merged.ClusterRefFields)
	assert.Equal(t, []string{FieldKeyVisionRef}, merged.LineageRefFields)
	assert.Equal(t, []string{"edge_doc"}, merged.ExclusiveKinds)
}

func TestShockwavePolicy_KindPredicates(t *testing.T) {
	t.Parallel()
	policy := ShockwavePolicy{
		LineageKinds:   []string{KindGoal, KindMilestone},
		ClusterKinds:   []string{KindRequirement, KindCriteria},
		ExclusiveKinds: []string{"edge_attachment"},
	}

	assert.True(t, policy.IsLineageKind(KindGoal))
	assert.True(t, policy.IsLineageKind("GOAL"))
	assert.False(t, policy.IsLineageKind(KindBacklogItem))

	assert.True(t, policy.IsClusterKind(KindRequirement))
	assert.True(t, policy.IsClusterKind("criteria"))
	assert.False(t, policy.IsClusterKind(KindGoal))

	assert.True(t, policy.IsExclusiveKind("edge_attachment"))
	assert.True(t, policy.IsExclusiveKind("EDGE_ATTACHMENT"))
	assert.False(t, policy.IsExclusiveKind("random_kind"))
}

func TestShockwavePolicy_HopRoleOverrides(t *testing.T) {
	t.Parallel()
	policy := ShockwavePolicy{
		LineageRefFields: []string{"custom_lineage_ref"},
		ClusterRefFields: []string{"custom_cluster_ref"},
	}

	assert.Equal(t, EdgeRoleMembership, policy.HopRole("any_kind", "custom_lineage_ref"))
	assert.True(t, policy.ShouldNoteLineage("any_kind", "custom_lineage_ref"))
	assert.False(t, policy.ShouldClusterHop("any_kind", "custom_lineage_ref"))

	assert.Equal(t, EdgeRoleComposition, policy.HopRole("any_kind", "custom_cluster_ref"))
	assert.True(t, policy.ShouldClusterHop("any_kind", "custom_cluster_ref"))
	assert.False(t, policy.ShouldNoteLineage("any_kind", "custom_cluster_ref"))
}
