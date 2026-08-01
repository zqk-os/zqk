package bldr_lifecycle_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/lifecycle_builders"
)

// PromptTemplateLifecycleBuilder builds the prompt_template lifecycle at version v1_0_0
// File: bldr_lifecycle_v1/prompt_template_builder.go - version is encoded in package/directory name
type PromptTemplateLifecycleBuilder struct {
	*lifecycle_builders.BaseLifecycleBuilder
}

// NewPromptTemplateLifecycleBuilder creates a new builder for prompt_template lifecycle version v1_0_0
func NewPromptTemplateLifecycleBuilder() *PromptTemplateLifecycleBuilder {
	builder := &PromptTemplateLifecycleBuilder{
		BaseLifecycleBuilder: lifecycle_builders.NewBaseLifecycleBuilder("prompt_template", "v1_0_0"),
	}

	// Add statuses and transitions
	builder.addPromptTemplateLifecycleData()

	return builder
}

// addPromptTemplateLifecycleData adds the prompt_template lifecycle statuses and transitions
func (b *PromptTemplateLifecycleBuilder) addPromptTemplateLifecycleData() {

	b.AddStatus(objects.Status{
		Value:   "draft",
		Display: "Draft",
		Origin:  true,
	})
	b.AddStatus(objects.Status{
		Value:   "active",
		Display: "Active",
	})
	b.AddStatus(objects.Status{
		Value:    "deprecated",
		Display:  "Deprecated",
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
		From:        "draft",
		To:          "active",
		Description: "Template reviewed and approved for general use",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "deprecated",
		Description: "Template superseded by a newer version but kept for reference",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "deprecated",
		To:          "archived",
		Description: "Template fully retired from active and fallback use",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "*",
		To:          "archived",
		Description: "Manual archival from any status",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "*",
		To:          "error",
		Description: "System-assigned when lifecycle incoherency or illegal transition is detected",
		Manual:      false,
		Auto:        false,
	})
}

func init() {
	lifecycle_builders.RegisterBuilder(NewPromptTemplateLifecycleBuilder())
}
