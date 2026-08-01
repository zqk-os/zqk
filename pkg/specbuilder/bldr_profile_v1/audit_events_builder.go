package bldr_profile_v1

import (
	"github.com/lanceman/zqk/pkg/config"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/profile_builders"
)

// AuditEventsBuilder builds the audit_events profile at version v1_0_0
// File: bldr_profile_v1/audit_events_builder.go - version is encoded in package/directory name
type AuditEventsBuilder struct {
	*profile_builders.BaseProfileBuilder
}

// NewAuditEventsBuilder creates a new builder for audit_events profile version v1_0_0
func NewAuditEventsBuilder() *AuditEventsBuilder {
	builder := &AuditEventsBuilder{
		BaseProfileBuilder: profile_builders.NewBaseProfileBuilder("audit_events", "v1_0_0"),
	}

	// Configure the profile
	builder.
		SetSchemaVersion(SchemaVersionV1).
		SetKind("profile").
		SetType(config.ProfileTypeMetricsSampler).
		SetMetadata(config.ProfileMetadata{
			Name:        "audit_events",
			Extends:     "base_sampler",
			Description: "Batches audit events by target object ID, creates metric every 100 events or 5 minutes.\\nStandard profile for audit events and change journal entries.\\n",
		}).
		SetSpec(map[string]any{
			objects.FieldKeyAppliesTo: []any{
				"audit_event",
				"change_journal_entry",
			},
			objects.FieldKeyBatchSize:       100,
			objects.FieldKeyFlushInterval:   "5m",
			objects.FieldKeyGroupByObjectID: true,
			objects.FieldKeyMaxBatchSize:    1000,
		})

	return builder
}

func init() {
	profile_builders.RegisterBuilder(NewAuditEventsBuilder())
}
