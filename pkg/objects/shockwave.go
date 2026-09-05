package objects

import "strings"

// Shockwave modes on a lifecycle status or transition.
// cluster: peer story graph; stop at lineage parents (child→parent refs).
// prune: archive the seed and recursively cascade objects that point at it (branch cut).
// shared: seed (+ exclusive edge kinds) only — fan-in lanes/views (workstream, roadmap, strategic_plan).
const (
	ShockwaveModeNone    = "none"
	ShockwaveModeCluster = "cluster"
	ShockwaveModePrune   = "prune"
	ShockwaveModeShared  = "shared"
	ShockwaveFailClosed  = "closed"
	ShockwaveFailOpen    = "open"
)

// ShockwavePolicy is composition on a lifecycle status/transition.
// Topology (cluster_ref_fields / lineage_ref_fields) is the spec graph; this is the hop policy.
type ShockwavePolicy struct {
	Mode             string   `yaml:"mode,omitempty"`
	Fail             string   `yaml:"fail,omitempty"` // closed (default) | open
	LineageKinds     []string `yaml:"lineage_kinds,omitempty"`
	ClusterKinds     []string `yaml:"cluster_kinds,omitempty"`
	ClusterRefFields []string `yaml:"cluster_ref_fields,omitempty"`
	LineageRefFields []string `yaml:"lineage_ref_fields,omitempty"`
	ExclusiveKinds   []string `yaml:"exclusive_kinds,omitempty"` // shared-mode edges that die with the seed
}

// MergeShockwavePolicy overlays child onto parent. Empty child fields inherit.
func MergeShockwavePolicy(parent, child ShockwavePolicy) ShockwavePolicy {
	out := parent
	if child.Mode != emptyValue {
		out.Mode = child.Mode
	}
	if child.Fail != emptyValue {
		out.Fail = child.Fail
	}
	if len(child.LineageKinds) > 0 {
		out.LineageKinds = child.LineageKinds
	}
	if len(child.ClusterKinds) > 0 {
		out.ClusterKinds = child.ClusterKinds
	}
	if len(child.ClusterRefFields) > 0 {
		out.ClusterRefFields = child.ClusterRefFields
	}
	if len(child.LineageRefFields) > 0 {
		out.LineageRefFields = child.LineageRefFields
	}
	if len(child.ExclusiveKinds) > 0 {
		out.ExclusiveKinds = child.ExclusiveKinds
	}
	return out
}

// DefaultArchiveShockwavePolicy is the cluster membrane: story kinds shockwave
// together; goal/milestone/mission/vision and shared lanes/views stay as trunks.
// priority_plan is also a lineage trunk (its status does not hop with a BLI seed),
// but archiving a member BLI while that plan is not archived is refused
// (RUL-1782235658105562000-b27c8dfc state ceiling). Park the plan to archive children.
func DefaultArchiveShockwavePolicy() ShockwavePolicy {
	return ShockwavePolicy{
		Mode: ShockwaveModeCluster,
		Fail: ShockwaveFailClosed,
		LineageKinds: []string{
			KindGoal, KindMilestone, KindMission, KindVision, KindPriorityPlan,
			KindWorkstream, KindRoadmap, KindStrategicPlan,
		},
		ClusterKinds: []string{
			KindCriteria, KindBacklogItem, KindRequirement,
		},
		ClusterRefFields: []string{
			FieldKeyCriteriaRefs, FieldKeyBacklogItemRefs, FieldKeyRequirementRefs,
		},
		LineageRefFields: []string{
			FieldKeyGoalRefs, FieldKeyMilestoneRefs, FieldKeyMissionRefs,
			FieldKeyVisionRef, FieldKeyPriorityPlanRef, FieldKeyPriorityPlanRefs,
			FieldKeyWorkstreamRef, FieldKeyWorkstreamRefs,
			FieldKeyFromWorkstreamRef, FieldKeyToWorkstreamRef,
		},
	}
}

// FillShockwavePolicyDefaults copies slice defaults when mode is set but lists were omitted.
func FillShockwavePolicyDefaults(p ShockwavePolicy) ShockwavePolicy {
	def := DefaultArchiveShockwavePolicy()
	if p.Fail == emptyValue {
		p.Fail = ShockwaveFailClosed
	}
	if len(p.LineageKinds) == 0 {
		p.LineageKinds = def.LineageKinds
	}
	if len(p.ClusterKinds) == 0 {
		p.ClusterKinds = def.ClusterKinds
	}
	if len(p.ClusterRefFields) == 0 {
		p.ClusterRefFields = def.ClusterRefFields
	}
	if len(p.LineageRefFields) == 0 {
		p.LineageRefFields = def.LineageRefFields
	}
	return p
}

// IsLineageKind reports whether kind is a lineage trunk or shared lane/view for this policy.
func (p ShockwavePolicy) IsLineageKind(kind string) bool {
	return stringInFold(p.LineageKinds, kind)
}

// IsClusterKind reports whether kind is in the story cluster.
func (p ShockwavePolicy) IsClusterKind(kind string) bool {
	return stringInFold(p.ClusterKinds, kind)
}

// IsExclusiveKind reports whether kind may hop with a shared-mode seed (edge object).
func (p ShockwavePolicy) IsExclusiveKind(kind string) bool {
	return stringInFold(p.ExclusiveKinds, kind)
}

// HopRole is the edge role shockwave consults for a ref field.
// Explicit lineage_ref_fields / cluster_ref_fields win; otherwise spec/DNA edge_role.
func (p ShockwavePolicy) HopRole(kind, field string) EdgeRole {
	if stringInFold(p.LineageRefFields, field) {
		return EdgeRoleMembership
	}
	if stringInFold(p.ClusterRefFields, field) {
		return EdgeRoleComposition
	}
	return EdgeRoleForKindField(kind, field, nil)
}

// ShouldClusterHop reports whether an outbound ref should expand the story cluster.
func (p ShockwavePolicy) ShouldClusterHop(kind, field string) bool {
	return p.HopRole(kind, field) == EdgeRoleComposition
}

// ShouldNoteLineage reports whether an outbound ref is a membership trunk to stop at.
func (p ShockwavePolicy) ShouldNoteLineage(kind, field string) bool {
	return p.HopRole(kind, field) == EdgeRoleMembership
}

func stringInFold(list []string, want string) bool {
	for _, s := range list {
		if strings.EqualFold(s, want) {
			return true
		}
	}
	return false
}
