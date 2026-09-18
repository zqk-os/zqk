package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// WatchdogRegistrationBuilder builds the watchdog_registration spec at version v2_0_0
// File: bldr_v2/watchdog_registration_builder.go - version is encoded in package/directory name
type WatchdogRegistrationBuilder struct {
	*builders.BaseSpecBuilder
}

// NewWatchdogRegistrationBuilder creates a new builder for watchdog_registration spec version v2_0_0
func NewWatchdogRegistrationBuilder() *WatchdogRegistrationBuilder {
	builder := &WatchdogRegistrationBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("watchdog_registration", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Registers an event subscription rule for the centralized Event-Driven Watchdog daemon. \\nInstead of hardcoding triggers or running independent polling loops, Agents or Agent Service Pools \\n(Teams) define their watch criteria here. When the target object kind meets the condition, the Watchdog \\nevaluates this rule and dispatches an audit_event payload to the specified `notify_target_ref` (typically \\nan `agent_feed`).\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addWatchdogRegistrationFields()

	return builder
}

// addWatchdogRegistrationFields adds the watchdog_registration fields
func (b *WatchdogRegistrationBuilder) addWatchdogRegistrationFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("condition_query", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("operator.").
			AutomationHooks("Parsed by Watchdog query builder.").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("Graph-state sync loop.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("The specific criteria to trigger the notification (e.g., 'status == error').").
			Security("non-sensitive").
			SystemUsage([]any{
				"watchdog",
			}).
			Validation("valid query string.").
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
		WithProfileCode("WDR-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("frequency", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("operator.").
			AutomationHooks("Sets the polling interval for the Watchdog scheduler.").
			Cardinality("one").
			Criticality("composition").
			Default("event-driven").
			Dependencies("Scheduler timing ticks.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("How frequently this rule should be evaluated (e.g., '1m', '5m', or 'event-driven').").
			Security("non-sensitive").
			SystemUsage([]any{
				"watchdog",
			}).
			Validation("duration format or enum.").
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
		WithProfileCode("WDR-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("heartbeat_enabled", "bool").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("operator.").
			AutomationHooks("Triggers all-clear events when queue is empty.").
			Cardinality("one").
			Criticality("composition").
			Default(false).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("If true, the Watchdog will periodically send an all-clear payload to assure the pool the monitoring loop is alive even if no infractions occur.").
			Security("non-sensitive").
			SystemUsage([]any{
				"watchdog",
			}).
			Validation("boolean.").
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
		WithProfileCode("WDR-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("note", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("operator.").
			AutomationHooks("none.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Human-readable context about why this registration exists.").
			Security("non-sensitive").
			SystemUsage([]any{
				"watchdog",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("WDR-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("notify_target_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system.").
			AutomationHooks("Watchdog routes events here.").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("Agent feeds or personas.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("The graph ID of the recipient (e.g., an agent_feed, team, or persona) that will receive the wake-up payload.").
			Security("non-sensitive").
			SystemUsage([]any{
				"watchdog",
			}).
			Validation("Valid ZQK reference ID.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("field_mutable_group", "field_reference_group", "reference").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("WDR-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("target_kind", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system.").
			AutomationHooks("Used by the Watchdog to filter graph queries.").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("Object kinds registry.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("The object kind being monitored (e.g., agent_task, convergence_session).").
			Security("non-sensitive").
			SystemUsage([]any{
				"watchdog",
			}).
			Validation("valid ZQK object kind.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MaxLength(64).
			Required(true).
			Build()).
		WithTraits("readable", "filterable").
		WithPermissions("r--").
		WithSemanticType("statement").
		WithProfileCode("WDR-001"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *WatchdogRegistrationBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *WatchdogRegistrationBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *WatchdogRegistrationBuilder) GetOntology() string {
	return "watchdog_registration"
}

func init() {
	builders.RegisterBuilder(NewWatchdogRegistrationBuilder())
}
