package trait_builders

import (
	"maps"

	"github.com/lanceman/zqk/pkg/objects"
)

// BaseTraitBuilder provides common functionality for trait builders
type BaseTraitBuilder struct {
	name        string
	version     string
	description string
	category    string
	objectLevel bool
	fieldLevel  bool
	requires    []string
	conflicts   []string
	includes    []string
	config      map[string]any
}

// NewBaseTraitBuilder creates a new base trait builder
func NewBaseTraitBuilder(name, version string) *BaseTraitBuilder {
	return &BaseTraitBuilder{
		name:      name,
		version:   version,
		category:  "standard", // Default category
		requires:  []string{},
		conflicts: []string{},
		includes:  []string{},
		config:    make(map[string]any),
	}
}

// SetDescription sets the description
func (b *BaseTraitBuilder) SetDescription(description string) *BaseTraitBuilder {
	b.description = description
	return b
}

// SetCategory sets the category
func (b *BaseTraitBuilder) SetCategory(category string) *BaseTraitBuilder {
	b.category = category
	return b
}

// SetObjectLevel sets object level flag
func (b *BaseTraitBuilder) SetObjectLevel(objectLevel bool) *BaseTraitBuilder {
	b.objectLevel = objectLevel
	return b
}

// SetFieldLevel sets field level flag
func (b *BaseTraitBuilder) SetFieldLevel(fieldLevel bool) *BaseTraitBuilder {
	b.fieldLevel = fieldLevel
	return b
}

// AddRequires adds a required trait
func (b *BaseTraitBuilder) AddRequires(trait string) *BaseTraitBuilder {
	b.requires = append(b.requires, trait)
	return b
}

// AddConflicts adds a conflicting trait
func (b *BaseTraitBuilder) AddConflicts(trait string) *BaseTraitBuilder {
	b.conflicts = append(b.conflicts, trait)
	return b
}

// AddIncludes adds an included trait (for trait groups)
func (b *BaseTraitBuilder) AddIncludes(trait string) *BaseTraitBuilder {
	b.includes = append(b.includes, trait)
	return b
}

// SetConfig sets the config map
func (b *BaseTraitBuilder) SetConfig(config map[string]any) *BaseTraitBuilder {
	b.config = config
	return b
}

// Build builds the trait definition
func (b *BaseTraitBuilder) Build() *objects.TraitDefinition {
	trait := &objects.TraitDefinition{
		Name:        b.name,
		Description: b.description,
		Category:    b.category,
		ObjectLevel: b.objectLevel,
		FieldLevel:  b.fieldLevel,
		Requires:    b.requires,
		Conflicts:   b.conflicts,
		Includes:    b.includes,
		Config:      b.config,
	}

	// Copy slices
	trait.Requires = make([]string, len(b.requires))
	copy(trait.Requires, b.requires)
	trait.Conflicts = make([]string, len(b.conflicts))
	copy(trait.Conflicts, b.conflicts)
	trait.Includes = make([]string, len(b.includes))
	copy(trait.Includes, b.includes)

	// Copy config map (defensive copy; trait literal above may alias b.config)
	trait.Config = make(map[string]any)
	maps.Copy(trait.Config, b.config)

	return trait
}

// GetVersion returns the version
func (b *BaseTraitBuilder) GetVersion() string {
	return b.version
}

// GetName returns the trait name
func (b *BaseTraitBuilder) GetName() string {
	return b.name
}
