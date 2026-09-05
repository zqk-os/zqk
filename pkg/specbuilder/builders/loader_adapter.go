package builders

import "github.com/lanceman/zqk/pkg/objects"

// SpecLoaderAdapter adapts VersionedBuilderRegistry to objects.VersionedBuilderRegistryInterface
// This allows pkg/objects to use builders without creating import cycles
type SpecLoaderAdapter struct {
	registry *VersionedBuilderRegistry
}

// NewSpecLoaderAdapter creates an adapter that implements objects.VersionedBuilderRegistryInterface
func NewSpecLoaderAdapter(registry *VersionedBuilderRegistry) *SpecLoaderAdapter {
	return &SpecLoaderAdapter{registry: registry}
}

// GetBuilder implements objects.VersionedBuilderRegistryInterface
func (a *SpecLoaderAdapter) GetBuilder(ontology, version string) (objects.SpecBuilderInterface, error) {
	builder, err := a.registry.GetBuilder(ontology, version)
	if err != nil {
		return nil, err
	}
	return &BuilderAdapter{builder: builder}, nil
}

// GetLatestVersion implements objects.VersionedBuilderRegistryInterface
func (a *SpecLoaderAdapter) GetLatestVersion(ontology string) (string, error) {
	return a.registry.GetLatestVersion(ontology)
}

// GetVersions implements objects.VersionedBuilderRegistryInterface
func (a *SpecLoaderAdapter) GetVersions(ontology string) []string {
	return a.registry.GetVersions(ontology)
}

// BuilderAdapter adapts SpecBuilder to objects.SpecBuilderInterface
type BuilderAdapter struct {
	builder SpecBuilder
}

// Build implements objects.SpecBuilderInterface
func (a *BuilderAdapter) Build() *objects.Spec {
	return a.builder.Build()
}

// GetVersion implements objects.SpecBuilderInterface
func (a *BuilderAdapter) GetVersion() string {
	return a.builder.GetVersion()
}

// GetOntology implements objects.SpecBuilderInterface
func (a *BuilderAdapter) GetOntology() string {
	return a.builder.GetOntology()
}

// InitializeGlobalSpecLoader sets up the builder registry on the global spec loader
// This should be called during application initialization
func InitializeGlobalSpecLoader() {
	registry := GetGlobalRegistry()
	adapter := NewSpecLoaderAdapter(registry)

	// Get the global spec loader and set the builder registry
	specLoader := objects.GetGlobalSpecLoader()
	specLoader.SetBuilderRegistry(adapter)
}
