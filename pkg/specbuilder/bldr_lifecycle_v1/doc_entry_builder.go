package bldr_lifecycle_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/lifecycle_builders"
)

// DocEntryLifecycleBuilder builds the doc_entry lifecycle at version v1_0_0
// File: bldr_lifecycle_v1/doc_entry_builder.go - version is encoded in package/directory name
type DocEntryLifecycleBuilder struct {
	*lifecycle_builders.BaseLifecycleBuilder
}

// NewDocEntryLifecycleBuilder creates a new builder for doc_entry lifecycle version v1_0_0
func NewDocEntryLifecycleBuilder() *DocEntryLifecycleBuilder {
	builder := &DocEntryLifecycleBuilder{
		BaseLifecycleBuilder: lifecycle_builders.NewBaseLifecycleBuilder("doc_entry", "v1_0_0"),
	}

	// Configure the lifecycle
	builder.
		SetPercentComplete(objects.PercentCompleteConfig{
			Method: "status_defaults",
			DefaultByStatus: map[string]any{
				"active":     100,
				"archived":   100,
				"deprecated": 100,
				"draft":      0,
				"error":      0,
				"published":  100,
				"review":     50,
			},
		})

	// Add statuses and transitions
	builder.addDocEntryLifecycleData()

	return builder
}

// addDocEntryLifecycleData adds the doc_entry lifecycle statuses and transitions
func (b *DocEntryLifecycleBuilder) addDocEntryLifecycleData() {

	b.AddStatus(objects.Status{
		Value:   "draft",
		Display: "Draft",
		Origin:  true,
	})
	b.AddStatus(objects.Status{
		Value:   "review",
		Display: "Review",
	})
	b.AddStatus(objects.Status{
		Value:    "published",
		Display:  "Published",
		Terminal: true,
	})
	b.AddStatus(objects.Status{
		Value:    "active",
		Display:  "Active",
		Terminal: true,
	})
	b.AddStatus(objects.Status{
		Value:    "archived",
		Display:  "Archived",
		Terminal: true,
		Archive:  true,
	})
	b.AddStatus(objects.Status{
		Value:    "deprecated",
		Display:  "Deprecated",
		Terminal: true,
		Archive:  true,
	})
	b.AddStatus(objects.Status{
		Value:   "error",
		Display: "Error",
		System:  true,
	})

	b.AddTransition(objects.Transition{
		From:        "draft",
		To:          "review",
		Description: "Document entry is submitted for review",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "review",
		To:          "published",
		Description: "Document entry is published",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "published",
		To:          "active",
		Description: "Document entry is activated (becomes searchable/visible)",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "*",
		To:          "archived",
		Description: "Document entry is archived (retention policy)",
		Manual:      false,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "*",
		To:          "deprecated",
		Description: "Document entry is deprecated (replaced or superseded)",
		Manual:      true,
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
	lifecycle_builders.RegisterBuilder(NewDocEntryLifecycleBuilder())
}
