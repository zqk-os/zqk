package bldr_v2

// Field name constants for goal objects
// These constants are generated from fields defined in THIS spec (not inherited)
// Inherited fields from base_object are defined in base_object_constants.go (same package)
// All constants are in package bldr_v2, so you can access inherited fields via bldr_v2.FieldX
const (
	// FieldAchievedAt is the field name for achieved_at
	FieldAchievedAt = "achieved_at"
	// FieldAuthority is the field name for authority
	FieldAuthority = "authority"
	// GoalFieldBacklogItemRefs is the field name for backlog_item_refs
	GoalFieldBacklogItemRefs = "backlog_item_refs"
	// GoalFieldCommitHashes is the field name for commit_hashes
	GoalFieldCommitHashes = "commit_hashes"
	// GoalFieldCommitRefs is the field name for commit_refs
	GoalFieldCommitRefs = "commit_refs" // Deprecated: use GoalFieldCommitHashes
	// FieldCurrentValue is the field name for current_value
	FieldCurrentValue = "current_value"
	// FieldMetric is the field name for metric
	FieldMetric = "metric"
	// FieldMetricTemplateId is the field name for metric_template_id
	FieldMetricTemplateId = "metric_template_id"
	// GoalFieldRequirementRefs is the field name for requirement_refs
	GoalFieldRequirementRefs = "requirement_refs"
	// GoalFieldTarget is the field name for target
	GoalFieldTarget = "target"
	// GoalFieldWorkstreamRefs is the field name for workstream_refs
	GoalFieldWorkstreamRefs = "workstream_refs"
	// GoalFieldEpicRefs is the field name for epic_refs
	GoalFieldEpicRefs = "epic_refs"
)
