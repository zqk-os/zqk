package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// CodeQualityMetricBuilder builds the code_quality_metric spec at version v2_0_0
// File: bldr_v2/code_quality_metric_builder.go - version is encoded in package/directory name
type CodeQualityMetricBuilder struct {
	*builders.BaseSpecBuilder
}

// NewCodeQualityMetricBuilder creates a new builder for code_quality_metric spec version v2_0_0
func NewCodeQualityMetricBuilder() *CodeQualityMetricBuilder {
	builder := &CodeQualityMetricBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("code_quality_metric", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_metric").
		SetDescription("Code quality metrics track policy enforcement and compliance over time. These metrics capture adherence to code quality policies (POL-####), measure technical debt trends, and provide observability into code quality maintenance effectiveness.\\nLifecycle: code_quality_metric_lifecycle.yaml.\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("listable").
		AddTrait("readable").
		AddTrait("writable").
		AddTrait("modifiable").
		AddTrait("formatable").
		AddTrait("groupable").
		AddTrait("filterable").
		AddTrait("sortable").
		AddTrait("searchable")

	// Add fields
	builder.addCodeQualityMetricFields()

	return builder
}

// addCodeQualityMetricFields adds the code_quality_metric fields
func (b *CodeQualityMetricBuilder) addCodeQualityMetricFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("compliance_percentage", "number").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (metric collector)").
			AutomationHooks("calculated from adherence metrics").
			Cardinality("zero_or_one").
			Criticality("composition").
			Default(nil).
			Dependencies("metric calculation system").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Compliance percentage (0-100) for adherence metrics").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"trend analysis",
				"compliance tracking",
			}).
			Validation("Float between 0 and 100").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("filterable", "readable", "sortable").
		WithPermissions("r-x").
		WithSemanticType("measurement").
		WithProfileCode("CQM-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("issue_count", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (metric collector)").
			AutomationHooks("populated by linter analysis").
			Cardinality("zero_or_one").
			Criticality("composition").
			Default(nil).
			Dependencies("linter system").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Number of issues detected (for violation metrics)").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"trend analysis",
				"compliance tracking",
			}).
			Validation("Non-negative integer").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("filterable", "readable", "sortable").
		WithPermissions("r-x").
		WithSemanticType("measurement").
		WithProfileCode("CQM-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("issue_type", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (metric collector)").
			AutomationHooks("populated by linter analysis").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("linter system").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Type of issue detected (e.g., \\\\\\\"gofmt\\\\\\\", \\\\\\\"unused\\\\\\\", \\\\\\\"unparam\\\\\\\", \\\\\\\"gocyclo\\\\\\\", \\\\\\\"gocritic\\\\\\\")").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"grouping",
				"reporting",
			}).
			Validation("Valid linter rule identifier").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MaxLength(100).
			Required(false).
			Build()).
		WithTraits("filterable", "groupable", "readable").
		WithPermissions("r-x").
		WithSemanticType("identifier").
		WithProfileCode("CQM-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("measurement_period", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (metric collector)").
			AutomationHooks("used for temporal analysis").
			Cardinality("one").
			Criticality("association").
			Default("daily").
			Dependencies("metric collection system").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Measurement period for the metric (daily, weekly, monthly)").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"grouping",
				"temporal analysis",
			}).
			Validation("Must be one of defined periods").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"daily",
				"weekly",
				"monthly",
			}).
			Required(true).
			Build()).
		WithTraits("filterable", "groupable", "readable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("CQM-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("metric_category", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (metric collector)").
			AutomationHooks("used for categorization, filtering").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("metric collection system").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Category of code quality metric (adherence, compliance, trend, violation, resolution)").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"grouping",
				"reporting",
			}).
			Validation("Must be one of defined categories").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"adherence",
				"compliance",
				"trend",
				"violation",
				"resolution",
			}).
			Required(true).
			Build()).
		WithTraits("filterable", "groupable", "readable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("CQM-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("policy_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (metric collector)").
			AutomationHooks("used for policy compliance tracking").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("policy registry").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Reference to policy being measured (POL-#### format, e.g., \\\\\\\"POL-EXAMPLE-001\\\\\\\")").
			Security("non-sensitive").
			SystemUsage([]any{
				"policy compliance",
				"filtering",
				"grouping",
				"reporting",
			}).
			Validation("Must match policy ID pattern (POL-####)").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^POL-[A-Z]+-\d{3}$`).
			Required(true).
			Build()).
		WithTraits("field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("CQM-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("resolution_count", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (metric collector)").
			AutomationHooks("populated from technical debt resolution tracking").
			Cardinality("zero_or_one").
			Criticality("composition").
			Default(nil).
			Dependencies("technical debt registry").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Number of technical debt items resolved (for resolution metrics)").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"trend analysis",
				"effectiveness tracking",
			}).
			Validation("Non-negative integer").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("filterable", "readable", "sortable").
		WithPermissions("r-x").
		WithSemanticType("measurement").
		WithProfileCode("CQM-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("tier", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (metric collector)").
			AutomationHooks("used for prioritization, filtering").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("metric collection system").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Issue tier for violation metrics (tier_1, tier_2, tier_3)").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"grouping",
				"prioritization",
			}).
			Validation("Must be one of defined tiers").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"tier_1",
				"tier_2",
				"tier_3",
			}).
			Required(false).
			Build()).
		WithTraits("filterable", "groupable", "readable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("CQM-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("trend_direction", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (metric collector)").
			AutomationHooks("calculated from historical metrics").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("trend analysis system").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Trend direction for trend metrics (improving, stable, degrading)").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"alerting",
				"analysis",
			}).
			Validation("Must be one of defined trend directions").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"improving",
				"stable",
				"degrading",
			}).
			Required(false).
			Build()).
		WithTraits("filterable", "groupable", "readable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("CQM-007"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *CodeQualityMetricBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *CodeQualityMetricBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *CodeQualityMetricBuilder) GetOntology() string {
	return "code_quality_metric"
}

func init() {
	builders.RegisterBuilder(NewCodeQualityMetricBuilder())
}
