package bldr_lifecycle_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/lifecycle_builders"
)

// ChangeJournalEntryLifecycleBuilder builds the change_journal_entry lifecycle at version v1_0_0
// File: bldr_lifecycle_v1/change_journal_entry_builder.go - version is encoded in package/directory name
type ChangeJournalEntryLifecycleBuilder struct {
	*lifecycle_builders.BaseLifecycleBuilder
}

// NewChangeJournalEntryLifecycleBuilder creates a new builder for change_journal_entry lifecycle version v1_0_0
func NewChangeJournalEntryLifecycleBuilder() *ChangeJournalEntryLifecycleBuilder {
	builder := &ChangeJournalEntryLifecycleBuilder{
		BaseLifecycleBuilder: lifecycle_builders.NewBaseLifecycleBuilder("change_journal_entry", "v1_0_0"),
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
	builder.addChangeJournalEntryLifecycleData()

	return builder
}

// addChangeJournalEntryLifecycleData adds the change_journal_entry lifecycle statuses and transitions
func (b *ChangeJournalEntryLifecycleBuilder) addChangeJournalEntryLifecycleData() {

	b.AddStatus(objects.Status{
		Value:   "pending",
		Display: "Pending",
		Origin:  true,
	})
	b.AddStatus(objects.Status{
		Value:   "completed",
		Display: "Completed",
	})
	b.AddStatus(objects.Status{
		Value:       "aggregated",
		Display:     "Aggregated",
		Terminal:    true,
		Description: "Entry has been aggregated into an aggregation metric (time window); eligible for compaction or cleanup.",
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
		Description: "Change journal entry operation completed successfully",
		Manual:      false,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "pending",
		To:          "failed",
		Description: "Change journal entry operation failed",
		Manual:      false,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "completed",
		To:          "reverted",
		Description: "Completed change journal entry is reverted",
		Manual:      false,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "completed",
		To:          "aggregated",
		Description: "Entry aggregated into metric (change_journal_aggregation job)",
		Manual:      false,
		Auto:        true,
	})

	b.AddTransition(objects.Transition{
		From:        "*",
		To:          "archived",
		Description: "Change journal entry is archived (retention policy)",
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
	lifecycle_builders.RegisterBuilder(NewChangeJournalEntryLifecycleBuilder())
}
