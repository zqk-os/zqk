package objects

import "strings"

// SpecKeyEdgeRole is the object-spec field key for a typed graph edge role.
// It lives on the field definition (sibling of semantic_type), not on instances.
const SpecKeyEdgeRole = "edge_role"

// EdgeRole is the first-class graph edge role query, shockwave, and agents consult.
// Storage stays one-owner / one-direction; the traverser uses role so parent-start
// and child-start walks are the same graph.
type EdgeRole string

const (
	// EdgeRoleMembership is child→parent “belongs to” (e.g. backlog_item.priority_plan_ref).
	EdgeRoleMembership EdgeRole = "membership"
	// EdgeRoleComposition is parent→child “completion set” (e.g. requirement.criteria_refs).
	EdgeRoleComposition EdgeRole = "composition"
	// EdgeRoleAssociate is an untyped or secondary link (e.g. related_object_refs).
	EdgeRoleAssociate EdgeRole = "associate"
)

// ParseEdgeRole returns a known role. checklist.criticality is not this enum.
func ParseEdgeRole(raw string) (EdgeRole, bool) {
	switch EdgeRole(strings.ToLower(strings.TrimSpace(raw))) {
	case EdgeRoleMembership:
		return EdgeRoleMembership, true
	case EdgeRoleComposition:
		return EdgeRoleComposition, true
	case EdgeRoleAssociate:
		return EdgeRoleAssociate, true
	default:
		return emptyValue, false
	}
}

// IsEdgeRoleName reports whether s is membership, composition, or associate.
func IsEdgeRoleName(s string) bool {
	_, ok := ParseEdgeRole(s)
	return ok
}

// GraphHop is one stored edge viewed from a node (outbound or inbound reverse).
type GraphHop struct {
	NeighborID string
	Field      string
	Role       EdgeRole
	Outbound   bool
}

// dnaFieldEdgeRoles is the runtime fallback when a spec field omits edge_role.
// Kind-specific rows win. Keep in lockstep with object_specs annotations
// .
var dnaFieldEdgeRoles = map[string]EdgeRole{
	FieldKeyPriorityPlanRef:   EdgeRoleMembership,
	FieldKeyMilestoneRef:      EdgeRoleMembership,
	FieldKeyCriteriaRefs:      EdgeRoleComposition,
	FieldKeyRelatedObjectRefs: EdgeRoleAssociate,
}

var dnaKindFieldEdgeRoles = map[string]map[string]EdgeRole{
	KindCriteria: {
		FieldKeyGoalRefs: EdgeRoleMembership,
	},
	KindBacklogItem: {
		FieldKeyGoalRefs:              EdgeRoleMembership,
		FieldKeyMilestoneRef:          EdgeRoleMembership,
		FieldKeyMilestoneRefs:         EdgeRoleMembership,
		FieldKeyRequirementRefs:       EdgeRoleMembership,
		FieldKeyConvergenceSessionRef: EdgeRoleMembership,
	},
	KindRequirement: {
		FieldKeyGoalRefs:       EdgeRoleMembership,
		FieldKeyMilestoneRefs:  EdgeRoleMembership,
		FieldKeyWorkstreamRefs: EdgeRoleMembership,
	},
	KindMilestone: {
		FieldKeyWorkstreamRefs: EdgeRoleMembership,
		FieldKeyGoalRefs:       EdgeRoleMembership,
	},
	KindTestCase: {
		FieldKeyRequirementRefs: EdgeRoleMembership,
	},
	KindTechnicalSpec: {
		FieldKeyRequirementRefs: EdgeRoleMembership,
	},
	KindAgentTask: {
		FieldKeyPipelineRef:    EdgeRoleMembership,
		FieldKeyBacklogItemRef: EdgeRoleMembership,
	},
	KindComponent: {
		FieldKeyDisplayRef:          EdgeRoleMembership,
		FieldKeyParentComponentRefs: EdgeRoleMembership,
	},
	KindTeam: {
		FieldKeyDepartmentRef: EdgeRoleMembership,
		FieldKeyDivisionRef:   EdgeRoleMembership,
	},
	KindImpactAnalysis: {
		FieldKeyChangeRef: EdgeRoleMembership,
	},
	KindPersona: {
		FieldKeyMissionRefs: EdgeRoleMembership,
	},
	KindPartnership: {
		FieldKeyOrganizationRefs: EdgeRoleComposition,
	},
	KindDivision: {
		FieldKeyParentDivisionRef: EdgeRoleMembership,
	},
}

// EdgeRoleFromFieldDef reads edge_role from a resolved spec field map.
func EdgeRoleFromFieldDef(fieldDef any) (EdgeRole, bool) {
	fieldMap, ok := fieldDef.(map[string]any)
	if !ok || fieldMap == nil {
		return emptyValue, false
	}
	raw, _ := fieldMap[SpecKeyEdgeRole].(string)
	return ParseEdgeRole(raw)
}

// EdgeRoleFromSpec reads edge_role from a loaded spec's resolved field.
func EdgeRoleFromSpec(spec *Spec, field string) (EdgeRole, bool) {
	if SpecResolvedFieldsMissing(spec) || field == emptyValue {
		return emptyValue, false
	}
	def, ok := spec.ResolvedFields[field]
	if !ok {
		return emptyValue, false
	}
	return EdgeRoleFromFieldDef(def)
}

// EdgeRoleForKindField resolves membership | composition | associate.
// Spec annotation wins, then kind+field DNA, then field DNA, else associate.
func EdgeRoleForKindField(kind, field string, spec *Spec) EdgeRole {
	if role, ok := EdgeRoleFromSpec(spec, field); ok {
		return role
	}
	if kind != emptyValue {
		if byField, ok := dnaKindFieldEdgeRoles[kind]; ok {
			if role, ok := byField[field]; ok {
				return role
			}
		}
	}
	if role, ok := dnaFieldEdgeRoles[field]; ok {
		return role
	}
	return EdgeRoleAssociate
}

// HopMatchesFilter reports whether a hop should be followed for GetRelated's
// relationshipType: empty (all), an EdgeRole name, or a concrete field name.
func HopMatchesFilter(field string, role EdgeRole, relationshipType string) bool {
	if relationshipType == emptyValue {
		return true
	}
	if parsed, ok := ParseEdgeRole(relationshipType); ok {
		return role == parsed
	}
	return field == relationshipType
}
