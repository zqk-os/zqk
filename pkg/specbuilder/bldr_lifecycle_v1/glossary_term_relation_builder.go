package bldr_lifecycle_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/lifecycle_builders"
)

// GlossaryTermRelationLifecycleBuilder builds the glossary_term_relation lifecycle at version v1_0_0
// File: bldr_lifecycle_v1/glossary_term_relation_builder.go - version is encoded in package/directory name
type GlossaryTermRelationLifecycleBuilder struct {
	*lifecycle_builders.BaseLifecycleBuilder
}

// NewGlossaryTermRelationLifecycleBuilder creates a new builder for glossary_term_relation lifecycle version v1_0_0
func NewGlossaryTermRelationLifecycleBuilder() *GlossaryTermRelationLifecycleBuilder {
	builder := &GlossaryTermRelationLifecycleBuilder{
		BaseLifecycleBuilder: lifecycle_builders.NewBaseLifecycleBuilder("glossary_term_relation", "v1_0_0"),
	}

	// Add statuses and transitions
	builder.addGlossaryTermRelationLifecycleData()

	return builder
}

// addGlossaryTermRelationLifecycleData adds the glossary_term_relation lifecycle statuses and transitions
func (b *GlossaryTermRelationLifecycleBuilder) addGlossaryTermRelationLifecycleData() {

	b.AddStatus(objects.Status{
		Value:       "active",
		Display:     "Active",
		Origin:      true,
		Description: "Edge is active",
	})
	b.AddStatus(objects.Status{
		Value:       "archived",
		Display:     "Archived",
		Terminal:    true,
		Archive:     true,
		Description: "Edge is retired",
	})
	b.AddStatus(objects.Status{
		Value:       "error",
		Display:     "Error",
		System:      true,
		Description: "System error",
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "archived",
		Description: "Archive relation",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "*",
		To:          "error",
		Description: "System error",
		Manual:      false,
		Auto:        true,
	})
}

func init() {
	lifecycle_builders.RegisterBuilder(NewGlossaryTermRelationLifecycleBuilder())
}
