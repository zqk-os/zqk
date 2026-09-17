package builders

import (
	"maps"

	"github.com/lanceman/zqk/pkg/objects"
)

// BaseSpecBuilder provides common functionality for spec builders
type BaseSpecBuilder struct {
	ontology      string
	version       string
	schemaVersion string
	extends       string
	composes      []string
	description   string
	visibility    string
	traits        []string
	fields        map[string]any
}

// NewBaseSpecBuilder creates a new base spec builder
func NewBaseSpecBuilder(ontology, version string) *BaseSpecBuilder {
	return &BaseSpecBuilder{
		ontology:      ontology,
		version:       version,
		schemaVersion: objects.DefaultSchemaVersion, // Default schema version
		visibility:    "internal",                   // Default visibility
		fields:        make(map[string]any),
		traits:        []string{},
	}
}

// SetSchemaVersion sets the schema version
func (b *BaseSpecBuilder) SetSchemaVersion(version string) *BaseSpecBuilder {
	b.schemaVersion = version
	return b
}

// SetExtends sets the extends field
func (b *BaseSpecBuilder) SetExtends(extends string) *BaseSpecBuilder {
	b.extends = extends
	return b
}

// AddCompose adds a mixin ontology merged after extends (see objects.Spec.Composes).
func (b *BaseSpecBuilder) AddCompose(ontology string) *BaseSpecBuilder {
	b.composes = append(b.composes, ontology)
	return b
}

// SetDescription sets the description
func (b *BaseSpecBuilder) SetDescription(description string) *BaseSpecBuilder {
	b.description = description
	return b
}

// SetVisibility sets the visibility
func (b *BaseSpecBuilder) SetVisibility(visibility string) *BaseSpecBuilder {
	b.visibility = visibility
	return b
}

// AddTrait adds a trait
func (b *BaseSpecBuilder) AddTrait(trait string) *BaseSpecBuilder {
	b.traits = append(b.traits, trait)
	return b
}

// AddField adds a field definition
func (b *BaseSpecBuilder) AddField(name string, fieldDef map[string]any) *BaseSpecBuilder {
	b.fields[name] = fieldDef
	return b
}

// AddFieldBuilder adds a field using a FieldBuilder
func (b *BaseSpecBuilder) AddFieldBuilder(fb *FieldBuilder) *BaseSpecBuilder {
	b.fields[fb.name] = fb.Build()
	return b
}

// Build builds the spec
func (b *BaseSpecBuilder) Build() *objects.Spec {
	// Copy traits
	traitsCopy := make([]string, len(b.traits))
	copy(traitsCopy, b.traits)

	// Copy fields
	fieldsCopy := make(map[string]any)
	maps.Copy(fieldsCopy, b.fields)

	spec := &objects.Spec{
		SchemaVersion: b.schemaVersion,
		Ontology:      b.ontology,
		Extends:       b.extends,
		Composes:      append([]string(nil), b.composes...),
		Visibility:    b.visibility,
		Description:   b.description,
		Traits:        traitsCopy,
		Fields:        fieldsCopy,
	}

	spec.ResolvedFields = make(map[string]any)
	maps.Copy(spec.ResolvedFields, spec.Fields)
	spec.ResolvedTraits = make([]string, len(spec.Traits))
	copy(spec.ResolvedTraits, spec.Traits)

	return spec
}

// GetVersion returns the version
func (b *BaseSpecBuilder) GetVersion() string {
	return b.version
}

// GetOntology returns the ontology
func (b *BaseSpecBuilder) GetOntology() string {
	return b.ontology
}
