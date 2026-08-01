package bldr_lifecycle_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/lifecycle_builders"
)

// AuditEventLifecycleBuilder builds the audit_event lifecycle at version v1_0_0
// File: bldr_lifecycle_v1/audit_event_builder.go - version is encoded in package/directory name
type AuditEventLifecycleBuilder struct {
	*lifecycle_builders.BaseLifecycleBuilder
}

// NewAuditEventLifecycleBuilder creates a new builder for audit_event lifecycle version v1_0_0
func NewAuditEventLifecycleBuilder() *AuditEventLifecycleBuilder {
	builder := &AuditEventLifecycleBuilder{
		BaseLifecycleBuilder: lifecycle_builders.NewBaseLifecycleBuilder("audit_event", "v1_0_0"),
	}

	// Configure the lifecycle
	builder.
		SetPercentComplete(objects.PercentCompleteConfig{
			Method: "status_defaults",
			DefaultByStatus: map[string]any{
				"archived":             100,
				"completed":            100,
				"error":                0,
				objects.FieldKeyFailed: 0,
				"pending":              0,
				"reverted":             0,
			},
		})

	// Add statuses and transitions
	builder.addAuditEventLifecycleData()

	return builder
}

// addAuditEventLifecycleData adds the audit_event lifecycle statuses and transitions
func (b *AuditEventLifecycleBuilder) addAuditEventLifecycleData() {

	b.AddStatus(objects.Status{
		Value:   "pending",
		Display: "Pending",
		Origin:  true,
	})
	b.AddStatus(objects.Status{
		Value:    "completed",
		Display:  "Completed",
		Terminal: true,
	})
	b.AddStatus(objects.Status{
		Value:    "failed",
		Display:  "Failed",
		Terminal: true,
	})
	b.AddStatus(objects.Status{
		Value:    "reverted",
		Display:  "Reverted",
		Terminal: true,
	})
	b.AddStatus(objects.Status{
		Value:    "archived",
		Display:  "Archived",
		Terminal: true,
		Archive:  true,
	})
	b.AddStatus(objects.Status{
		Value:   "error",
		Display: "Error",
		System:  true,
	})

	b.AddTransition(objects.Transition{
		From:        "pending",
		To:          "completed",
		Description: "Audit event operation completed successfully",
		Manual:      false,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "pending",
		To:          "failed",
		Description: "Audit event operation failed",
		Manual:      false,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "completed",
		To:          "reverted",
		Description: "Completed audit event is reverted",
		Manual:      false,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "*",
		To:          "archived",
		Description: "Audit event is archived (retention policy)",
		Manual:      false,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "*",
		To:          "error",
		Description: "System error occurred",
		Manual:      false,
		Auto:        true,
	})
}

func init() {
	lifecycle_builders.RegisterBuilder(NewAuditEventLifecycleBuilder())
}
