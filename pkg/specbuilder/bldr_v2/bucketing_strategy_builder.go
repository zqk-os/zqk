package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// BucketingStrategyBuilder builds the bucketing_strategy spec at version v2_0_0
// File: bldr_v2/bucketing_strategy_builder.go - version is encoded in package/directory name
type BucketingStrategyBuilder struct {
	*builders.BaseSpecBuilder
}

// NewBucketingStrategyBuilder creates a new builder for bucketing_strategy spec version v2_0_0
func NewBucketingStrategyBuilder() *BucketingStrategyBuilder {
	builder := &BucketingStrategyBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("bucketing_strategy", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("auditable").
		SetDescription("Base specification for bucketing strategy objects. Bucketing strategies define how high-volume objects are organized into buckets (subdirectories) for efficient storage and retrieval. Multiple strategies can be active concurrently for the same object kind.\\nLifecycle: bucketing_strategy_lifecycle.yaml.\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion)

	// Add fields
	builder.addBucketingStrategyFields()

	return builder
}

// addBucketingStrategyFields adds the bucketing_strategy fields
func (b *BucketingStrategyBuilder) addBucketingStrategyFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("applies_to", "array").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("determines which object kinds use this strategy.").
			Cardinality("many").
			Criticality("composition").
			Default([]any{
				"graph_node",
				objects.KindAuditEvent,
			}).
			Dependencies("object kind registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("List of object kinds this strategy applies to (e.g., [\\\\\\\"audit_event\\\\\\\", \\\\\\\"change_journal_entry\\\\\\\"]).").
			Security("non-sensitive").
			SystemUsage([]any{
				"strategy routing",
				"hash registry management",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithTraits("readable", "writable", "filterable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("BST-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("archive_strategy", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("used by archive scheduler jobs to determine archival behavior.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("bucketing strategy must be enabled.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Archive strategy configuration for long-term storage. Supports simple single-tier or complex multi-tier progression (warm → cold → iced).").
			Security("non-sensitive").
			SystemUsage([]any{
				"archive scheduling",
				"storage tier management",
				"lifecycle management",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("BST-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("enabled", "boolean").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("determines if strategy is active.").
			Cardinality("one").
			Criticality("metadata").
			Default(false).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Whether this strategy is currently enabled and active.").
			Security("non-sensitive").
			SystemUsage([]any{
				"strategy activation",
				"runtime configuration",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithTraits("readable", "writable", "filterable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("BST-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("field", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("used to extract bucket key from object data.").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("strategy_type determines which field is used.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Object field name to extract bucket key from (e.g., \\\\\\\"created_at\\\\\\\", \\\\\\\"status\\\\\\\", \\\\\\\"size\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"bucket key extraction",
				"storage organization",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithTraits("readable", "writable", "filterable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("BST-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("format", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("used for chronological strategies to format timestamps.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("required for chronological strategies.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Go time format string for chronological strategies. Common formats: \\\\\\\"2006-01\\\\\\\" (monthly), \\\\\\\"2006-01-02\\\\\\\" (daily), \\\\\\\"2006-01-02T15\\\\\\\" (hourly), \\\\\\\"2006-01-02T15:04\\\\\\\" (half_hourly), \\\\\\\"2006-01-02T15:04:05\\\\\\\" (qtr_hourly), \\\\\\\"2006-01-02T15:04:05.9\\\\\\\" (tenths). Or use predefined granularity: \\\\\\\"monthly\\\\\\\", \\\\\\\"weekly\\\\\\\", \\\\\\\"daily\\\\\\\", \\\\\\\"hourly\\\\\\\", \\\\\\\"half_hourly\\\\\\\", \\\\\\\"qtr_hourly\\\\\\\", \\\\\\\"tenths\\\\\\\".").
			Security("non-sensitive").
			SystemUsage([]any{
				"timestamp formatting",
				"bucket key generation",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("BST-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("retention_tolerance", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("used by retention_tolerance scheduler job to trigger archive and cleanup per kind.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("bucketing strategy applies_to kinds.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Configurable tolerance that triggers archive and cleanup when retention_tolerance job runs.\narchive_after (duration, e.g. \\\"24h\\\", \\\"30d\\\") - objects older than this are marked status=archived.\ncleanup_after (duration) - objects older than this are deleted (only if status not in protect_statuses).\nmax_count (integer, 0=no limit) - oldest objects not in protect_statuses are deleted until count <= max_count.\nprotect_statuses (array of strings) - active-like statuses that must not be removed; strategy does not delete objects in these statuses. If count cannot be reduced (all objects protected), job returns a blocking error requiring human intervention.\nAligns with docs/process/_internal/configs/retention_tolerance.yaml; strategy-level value overrides config for applies_to kinds.\n").
			Security("non-sensitive").
			SystemUsage([]any{
				"retention_tolerance job",
				"archive and cleanup scheduling",
				"object count limits",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("BST-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("strategy_name", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("used for strategy identification and logging.").
			Cardinality("one").
			Criticality("metadata").
			Default(nil).
			Dependencies("strategy_type.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Human-readable name for this strategy instance (e.g., \\\\\\\"monthly\\\\\\\", \\\\\\\"status_based\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"strategy identification",
				"logging",
				"debugging",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithTraits("readable", "writable", "filterable", "searchable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("BST-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("strategy_type", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system architect").
			AutomationHooks("used by storage layer to determine bucket organization.").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("storage system.").
			Lifecycle("mutable (can be updated as system evolves)").
			Observability("yes").
			Purpose("Type of bucketing strategy (chronological, state, size, composite, first_letter). first_letter buckets by first character of a string field (e.g. title); used for glossary_term (A -> a, P -> p, etc.).").
			Security("non-sensitive").
			SystemUsage([]any{
				"storage organization",
				"hash registry management",
				"performance optimization",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithTraits("readable", "writable", "filterable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("BST-001"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *BucketingStrategyBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *BucketingStrategyBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *BucketingStrategyBuilder) GetOntology() string {
	return "bucketing_strategy"
}

func init() {
	builders.RegisterBuilder(NewBucketingStrategyBuilder())
}
