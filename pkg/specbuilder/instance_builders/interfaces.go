package instance_builders

import (
	"github.com/zqk-os/zqk/pkg/errfmt"
)

// InstanceBuilder is an interface for builders that create and manage object instances
// Unlike spec builders (one per spec), there's ONE instance builder per object type
// that can handle many instances of that type
type InstanceBuilder interface {
	// Build creates a new instance map (fluent API construction)
	Build() (map[string]any, error)
	// GetKind returns the object kind this builder handles
	GetKind() string
	// GetSchemaVersion returns the schema version this builder supports (e.g., objects.DefaultSchemaVersion)
	GetSchemaVersion() string

	// SetField sets a field value (for buffering values during construction)
	SetField(fieldName string, value any) InstanceBuilder

	// SetID sets the ID field (convenience method).
	SetID(id string) InstanceBuilder

	// SetStatus sets the status field (convenience method).
	SetStatus(status string) InstanceBuilder

	// LoadFromYAML loads an instance from a YAML file
	LoadFromYAML(yamlPath string) (map[string]any, error)

	// LoadFromSequence loads an instance from a sequence file (compact format: values in order)
	LoadFromSequence(sequencePath string) (map[string]any, error)

	// WriteToYAML writes an instance to a YAML file
	WriteToYAML(instance map[string]any, yamlPath string) error

	// WriteToSequence writes an instance to a sequence file (compact format)
	WriteToSequence(instance map[string]any, sequencePath string) error
}

// SpecBuilderInterface allows instance builders to reference spec builders without import cycles
// This interface should be implemented by pkg/specbuilder/builders.SpecBuilder
type SpecBuilderInterface interface {
	Build() any // Returns *objects.Spec but using any to avoid import cycles
	GetVersion() string
	GetOntology() string
}

// VersionedInstanceBuilderRegistry manages versioned instance builders
type VersionedInstanceBuilderRegistry struct {
	builders     map[string]map[string]InstanceBuilder // kind -> schema_version -> builder
	specRegistry SpecBuilderRegistryInterface          // Reference to spec builder registry
}

// SpecBuilderRegistryInterface allows instance builders to use spec builder registry without import cycles
type SpecBuilderRegistryInterface interface {
	GetBuilder(ontology, version string) (SpecBuilderInterface, error)
	GetLatestVersion(ontology string) (string, error)
}

// NewVersionedInstanceBuilderRegistry creates a new instance builder registry
func NewVersionedInstanceBuilderRegistry(specRegistry SpecBuilderRegistryInterface) *VersionedInstanceBuilderRegistry {
	return &VersionedInstanceBuilderRegistry{
		builders:     make(map[string]map[string]InstanceBuilder),
		specRegistry: specRegistry,
	}
}

// Register registers an instance builder
func (r *VersionedInstanceBuilderRegistry) Register(builder InstanceBuilder) {
	kind := builder.GetKind()
	version := builder.GetSchemaVersion()

	if r.builders[kind] == nil {
		r.builders[kind] = make(map[string]InstanceBuilder)
	}
	r.builders[kind][version] = builder
}

// GetBuilder returns the builder for a specific kind and schema version
func (r *VersionedInstanceBuilderRegistry) GetBuilder(kind, schemaVersion string) (InstanceBuilder, error) {
	if r.builders[kind] == nil {
		return nil, errfmt.Errorf("no builders found for kind: %s", kind)
	}
	builder, ok := r.builders[kind][schemaVersion]
	if !ok {
		return nil, errfmt.Errorf("no builder found for kind %s at version %s", kind, schemaVersion)
	}
	return builder, nil
}

// GetLatestVersion returns the latest schema version for a kind
func (r *VersionedInstanceBuilderRegistry) GetLatestVersion(kind string) (string, error) {
	if len(r.builders[kind]) == 0 {
		return "", errfmt.Errorf("no builders found for kind: %s", kind)
	}

	// Find latest version (assuming semantic versioning)
	latest := ""
	for version := range r.builders[kind] {
		if latest == emptyValue || compareVersions(version, latest) > 0 {
			latest = version
		}
	}
	return latest, nil
}

// GetAllKinds returns all kinds that have instance builders
func (r *VersionedInstanceBuilderRegistry) GetAllKinds() []string {
	kinds := make([]string, 0, len(r.builders))
	for kind := range r.builders {
		kinds = append(kinds, kind)
	}
	return kinds
}

// compareVersions compares two semantic version strings (major.minor.patch).
// For the canonical default object schema version, see pkg/objects.DefaultSchemaVersion.
// Returns: >0 if v1 > v2, 0 if v1 == v2, <0 if v1 < v2
func compareVersions(v1, v2 string) int {
	// Simple comparison - for proper semantic versioning, use a library
	// For now, assume format "major.minor.patch"
	// This is a simplified comparison
	if v1 == v2 {
		return 0
	}
	// For simplicity, just compare strings (works for most cases)
	if v1 > v2 {
		return 1
	}
	return -1
}
