package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// ScenarioBuilder builds the scenario spec at version v2_0_0
// File: bldr_v2/scenario_builder.go - version is encoded in package/directory name
type ScenarioBuilder struct {
	*builders.BaseSpecBuilder
}

// NewScenarioBuilder creates a new builder for scenario spec version v2_0_0
func NewScenarioBuilder() *ScenarioBuilder {
	builder := &ScenarioBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("scenario", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Mock project scenarios / labs used to exercise tooling edge cases. ").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addScenarioFields()

	return builder
}

// addScenarioFields adds the scenario fields
func (b *ScenarioBuilder) addScenarioFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("category", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for scenario categorization.").
			Cardinality("one").
			Criticality("association").
			Default("").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Category classification (e.g., \\\\\\\"performance\\\\\\\", \\\\\\\"validation\\\\\\\", \\\\\\\"integration\\\\\\\", \\\\\\\"stress\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"grouping",
				"organization",
			}).
			Validation("freeform text but encourage controlled vocabulary.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable", "filterable", "groupable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("SCN-014"))
	b.AddFieldBuilder(builders.NewFieldBuilder("change_policy", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used by scenario builder to handle object changes during snapshot.").
			Cardinality("one").
			Criticality("association").
			Default("include").
			Dependencies("scenario builder.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Policy for handling objects that change between snapshot initiation and clone completion. Options: \\\\\\\"reject\\\\\\\" (skip changed objects), \\\\\\\"include\\\\\\\" (use current state), \\\\\\\"reconstruct\\\\\\\" (reconstruct from change journal).").
			Security("non-sensitive").
			SystemUsage([]any{
				"change handling",
				"snapshot integrity",
			}).
			Validation("Must be one of [\\\\\\\"reject\\\\\\\", \\\\\\\"include\\\\\\\", \\\\\\\"reconstruct\\\\\\\"].").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("SCN-022"))
	b.AddFieldBuilder(builders.NewFieldBuilder("cleanup_config", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used by cleanup commands to determine what to clean.").
			Cardinality("one").
			Criticality("composition").
			Default(map[string]any{}).
			Dependencies("cleanup tools.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Cleanup configuration (what to preserve, what to clean, retention policies, etc.).").
			Security("non-sensitive").
			SystemUsage([]any{
				"scenario cleanup",
				"data management",
			}).
			Validation("YAML object with cleanup settings.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("SCN-011"))
	b.AddFieldBuilder(builders.NewFieldBuilder("data_generation_config", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used by scenario builder to generate test data.").
			Cardinality("one").
			Criticality("composition").
			Default(map[string]any{}).
			Dependencies("scenario builder.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Configuration for automated test data generation (kinds, counts, diversity, etc.).").
			Security("non-sensitive").
			SystemUsage([]any{
				"test data generation",
				"scenario recreation",
			}).
			Validation("YAML object with data generation settings.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("SCN-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("documentation_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for documentation linking.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("documentation registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to documentation files (doc_entry IDs) related to this scenario.").
			Security("non-sensitive").
			SystemUsage([]any{
				"documentation",
				"traceability",
			}).
			Validation("Must reference existing doc_entry IDs.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("SCN-017"))
	b.AddFieldBuilder(builders.NewFieldBuilder("edge_cases", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("ensures coverage metrics.").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("coverage reporting.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Enumerate edge cases covered.").
			Security("non-sensitive").
			SystemUsage([]any{
				"test planning",
			}).
			Validation("freeform text but encourage structured tags.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("SCN-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("environment_config", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used to set environment variables and resource limits.").
			Cardinality("one").
			Criticality("composition").
			Default(map[string]any{}).
			Dependencies("test runners.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Environment configuration (environment variables, resource limits, timeouts, etc.).").
			Security("non-sensitive").
			SystemUsage([]any{
				"test execution",
				"resource management",
			}).
			Validation("YAML object with environment settings.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("SCN-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("fixtures", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used by scenario runners.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("fixture directory structure.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Paths to fixture data/assets.").
			Security("non-sensitive").
			SystemUsage([]any{
				"scenario runners",
			}).
			Validation("paths must exist or be resolvable.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("SCN-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("goal_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for goal-scenario traceability.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("goal registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Goals this scenario tests.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
			}).
			Validation("Must reference existing goal IDs.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("SCN-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("hash_mappings", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system.").
			AutomationHooks("created automatically by scenario builder during copy.").
			Cardinality("one").
			Criticality("composition").
			Default(map[string]any{}).
			Dependencies("CAS system.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Hash mapping from original object hash to new scenario object hash (original_hash -> new_hash). Used for integrity verification and reference updates.").
			Security("non-sensitive").
			SystemUsage([]any{
				"integrity verification",
				"reference updates",
			}).
			Validation("YAML object mapping original hashes to new hashes.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("SCN-021"))
	b.AddFieldBuilder(builders.NewFieldBuilder("infrastructure_config", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used by scenario builder to determine what infrastructure to copy.").
			Cardinality("one").
			Criticality("composition").
			Default(map[string]any{}).
			Dependencies("scenario builder.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Configuration for infrastructure setup (which configs, specs, lifecycles to copy, or use defaults).").
			Security("non-sensitive").
			SystemUsage([]any{
				"infrastructure setup",
				"scenario initialization",
			}).
			Validation("YAML object with infrastructure settings (configs, specs, lifecycles, profiles).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("SCN-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("last_run_info", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system.").
			AutomationHooks("updated automatically by scenario runners.").
			Cardinality("one").
			Criticality("association").
			Default(map[string]any{}).
			Dependencies("scenario runners.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Information about the last scenario execution (timestamp, duration, status, metrics, etc.).").
			Security("non-sensitive").
			SystemUsage([]any{
				"execution tracking",
				"metrics",
			}).
			Validation("YAML object with execution info.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "modifiable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("SCN-018"))
	b.AddFieldBuilder(builders.NewFieldBuilder("objective", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used by scenario runners.").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("mock project manifests.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("What the scenario aims to test.").
			Security("non-sensitive").
			SystemUsage([]any{
				"testing",
				"documentation",
			}).
			Validation("markdown allowed.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("SCN-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("performance_targets", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for performance validation and reporting.").
			Cardinality("one").
			Criticality("association").
			Default(map[string]any{}).
			Dependencies("performance monitoring.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Performance targets and thresholds (e.g., max execution time, min cache hit rate, etc.).").
			Security("non-sensitive").
			SystemUsage([]any{
				"performance validation",
				"reporting",
			}).
			Validation("YAML object with performance metrics and thresholds.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("SCN-015"))
	b.AddFieldBuilder(builders.NewFieldBuilder("scenario_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for scenario dependency management.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("scenario registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Other scenarios this scenario depends on or extends.").
			Security("non-sensitive").
			SystemUsage([]any{
				"dependency management",
				"scenario composition",
			}).
			Validation("Must reference existing scenario IDs.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("SCN-012"))
	b.AddFieldBuilder(builders.NewFieldBuilder("scheduler_config", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used to configure scheduler for test scenario.").
			Cardinality("one").
			Criticality("composition").
			Default(map[string]any{}).
			Dependencies("scheduler.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Scheduler configuration for the scenario (enabled, project_type, job overrides, etc.).").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduler setup",
				"job configuration",
			}).
			Validation("YAML object with scheduler settings.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("SCN-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("snapshot_hashes", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system.").
			AutomationHooks("captured automatically by scenario builder during snapshot.").
			Cardinality("one").
			Criticality("composition").
			Default(map[string]any{}).
			Dependencies("CAS system.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Hash of each source object at snapshot time (object_id -> hash). Used for integrity verification and change detection.").
			Security("non-sensitive").
			SystemUsage([]any{
				"integrity verification",
				"change detection",
				"state reconstruction",
			}).
			Validation("YAML object mapping object IDs to hashes.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("SCN-020"))
	b.AddFieldBuilder(builders.NewFieldBuilder("snapshot_timestamp", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system.").
			AutomationHooks("set by snapshot-scenario command during capture.").
			Cardinality("one").
			Criticality("high").
			Default(nil).
			Dependencies("snapshot system.").
			Lifecycle("immutable after capture.").
			Observability("yes").
			Purpose("Timestamp (UTC) at which the system state was captured for this scenario. Used for change detection and historical state reconstruction.").
			Security("non-sensitive").
			SystemUsage([]any{
				"state reconstruction",
				"change detection",
				"temporal analysis",
			}).
			Validation("Must be a valid UTC datetime string (RFC3339Nano).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$`).
			Required(false).
			Build()).
		WithTraits("readable").
		WithPermissions("r-x").
		WithSemanticType("datetime").
		WithProfileCode("SCN-019"))
	b.AddFieldBuilder(builders.NewFieldBuilder("source_object_ids", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used by scenario builder when copying objects from project.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("object registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Object IDs from the main project that were copied to create this scenario (for traceability and recreation).").
			Security("non-sensitive").
			SystemUsage([]any{
				"scenario recreation",
				"traceability",
			}).
			Validation("Must reference existing object IDs in the source project.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("SCN-019"))
	b.AddFieldBuilder(builders.NewFieldBuilder("tags", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for scenario filtering and organization.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Tags for categorizing and filtering scenarios (e.g., \\\\\\\"performance\\\\\\\", \\\\\\\"validation\\\\\\\", \\\\\\\"integration\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"organization",
				"discovery",
			}).
			Validation("freeform text tags.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable", "filterable", "groupable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("SCN-013"))
	b.AddFieldBuilder(builders.NewFieldBuilder("test_execution_config", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used by test runners to execute scenario tests.").
			Cardinality("one").
			Criticality("composition").
			Default(map[string]any{}).
			Dependencies("test runners.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Test execution configuration (commands to run, assertions, performance targets, etc.).").
			Security("non-sensitive").
			SystemUsage([]any{
				"test execution",
				"validation",
			}).
			Validation("YAML object with test execution settings.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("SCN-010"))
	b.AddFieldBuilder(builders.NewFieldBuilder("validation_rules", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for automated validation checks.").
			Cardinality("one").
			Criticality("composition").
			Default(map[string]any{}).
			Dependencies("validation system.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Validation rules that must pass for scenario to be considered successful (e.g., no blocking errors, max warning count, etc.).").
			Security("non-sensitive").
			SystemUsage([]any{
				"test validation",
				"quality gates",
			}).
			Validation("YAML object with validation rules.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("SCN-016"))
	b.AddFieldBuilder(builders.NewFieldBuilder("workstream_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for workstream-scenario traceability.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("workstream registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Workstreams this scenario exercises.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
			}).
			Validation("Must reference existing workstream IDs.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("SCN-005"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *ScenarioBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *ScenarioBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *ScenarioBuilder) GetOntology() string {
	return "scenario"
}

func init() {
	builders.RegisterBuilder(NewScenarioBuilder())
}
