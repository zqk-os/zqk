package builders

import (
	"fmt"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
)

// SpecBuilder is an interface for builders that generate object specs at a specific version
type SpecBuilder interface {
	// Build returns the spec at this builder's version
	Build() *objects.Spec
	// GetVersion returns the version this builder generates
	GetVersion() string
	// GetOntology returns the ontology/name of the spec this builder generates
	GetOntology() string
}

// VersionedBuilderRegistry manages versioned builders
type VersionedBuilderRegistry struct {
	builders map[string]map[string]SpecBuilder // ontology -> version -> builder
}

// NewVersionedBuilderRegistry creates a new registry
func NewVersionedBuilderRegistry() *VersionedBuilderRegistry {
	return &VersionedBuilderRegistry{
		builders: make(map[string]map[string]SpecBuilder),
	}
}

// Register registers a builder for a specific ontology and version
func (r *VersionedBuilderRegistry) Register(builder SpecBuilder) {
	ontology := builder.GetOntology()
	version := builder.GetVersion()

	if r.builders[ontology] == nil {
		r.builders[ontology] = make(map[string]SpecBuilder)
	}
	r.builders[ontology][version] = builder
}

// GetBuilder returns the builder for a specific ontology and version
func (r *VersionedBuilderRegistry) GetBuilder(ontology, version string) (SpecBuilder, error) {
	if r.builders[ontology] == nil {
		return nil, errfmt.Errorf("no builders found for ontology: %s", ontology)
	}

	// Normalize version (e.g., "2.0.0" -> "v2_0_0")
	normalizedVersion := ParseInstanceVersion(version)

	builder, ok := r.builders[ontology][normalizedVersion]
	if !ok {
		// Fallback: try raw version if normalization didn't help (or if it was already normalized)
		builder, ok = r.builders[ontology][version]
		if !ok {
			return nil, errfmt.Errorf("no builder found for ontology %s at version %s (tried %s and %s)",
				ontology, version, normalizedVersion, version)
		}
	}
	return builder, nil
}

// GetLatestVersion returns the latest version for an ontology
// Versions are expected to be in semantic format: "v1_0_0", "v1_1_0", "v2_0_0", etc.
func (r *VersionedBuilderRegistry) GetLatestVersion(ontology string) (string, error) {
	if len(r.builders[ontology]) == 0 {
		return "", errfmt.Errorf("no builders found for ontology: %s", ontology)
	}

	// Find highest semantic version
	latestVersion := ""
	var maxMajor, maxMinor, maxPatch = -1, -1, -1

	for version := range r.builders[ontology] {
		// Parse semantic version (format "v1_0_0" -> major=1, minor=0, patch=0)
		var major, minor, patch int
		if _, err := fmt.Sscanf(version, "v%d_%d_%d", &major, &minor, &patch); err == nil {
			// Compare semantic versions (major.minor.patch)
			if major > maxMajor ||
				(major == maxMajor && minor > maxMinor) ||
				(major == maxMajor && minor == maxMinor && patch > maxPatch) {
				maxMajor = major
				maxMinor = minor
				maxPatch = patch
				latestVersion = version
			}
		}
	}

	if latestVersion == emptyValue {
		// Fallback: return any version if parsing failed
		for version := range r.builders[ontology] {
			return version, nil
		}
	}

	return latestVersion, nil
}

// GetAllOntologies returns all ontologies with builders
func (r *VersionedBuilderRegistry) GetAllOntologies() []string {
	ontologies := make([]string, 0, len(r.builders))
	for ontology := range r.builders {
		ontologies = append(ontologies, ontology)
	}
	return ontologies
}

// GetVersions returns all versions for an ontology
func (r *VersionedBuilderRegistry) GetVersions(ontology string) []string {
	if r.builders[ontology] == nil {
		return nil
	}
	versions := make([]string, 0, len(r.builders[ontology]))
	for version := range r.builders[ontology] {
		versions = append(versions, version)
	}
	return versions
}
