package instance_builders

import (
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	sbyaml "github.com/zqk-os/zqk/pkg/specbuilder/yaml"
)

const instanceFileExtYAML = ".yaml"

// InstanceGenerator generates instance files from instance builders
type InstanceGenerator struct {
	registry  *VersionedInstanceBuilderRegistry
	outputDir string
	writer    *sbyaml.YAMLWriter[map[string]any]
}

// NewInstanceGenerator creates a new instance generator
func NewInstanceGenerator(outputDir string, registry *VersionedInstanceBuilderRegistry) *InstanceGenerator {
	return &InstanceGenerator{
		registry:  registry,
		outputDir: outputDir,
		writer:    sbyaml.NewYAMLWriter[map[string]any](),
	}
}

// GenerateInstance generates an instance file for a specific kind, ID, and version
func (ig *InstanceGenerator) GenerateInstance(kind, id, schemaVersion string) error {
	builder, err := ig.registry.GetBuilder(kind, schemaVersion)
	if err != nil {
		return errfmt.Errorf("failed to get builder for %s@%s: %w", kind, schemaVersion, err)
	}

	// Set ID before build to avoid "id is required" validation error
	builder.SetID(id)
	instance, err := builder.Build()
	if err != nil {
		return errfmt.Newf("failed to build instance").Wrap(err)
	}

	// Ensure ID matches
	if instanceID, ok := instance[objects.FieldKeyID].(string); ok && instanceID != id {
		return errfmt.Errorf("instance ID mismatch: expected %s, got %s", id, instanceID)
	}

	// Generate filename from ID
	filename := id + instanceFileExtYAML
	filePath := filepath.Join(ig.outputDir, filename)

	if err := ig.writer.WriteToFile(instance, filePath); err != nil {
		return errfmt.Newf("failed to write instance file").Wrap(err)
	}

	return nil
}

// GenerateLatestInstance generates the latest version of an instance
func (ig *InstanceGenerator) GenerateLatestInstance(kind, id string) error {
	version, err := ig.registry.GetLatestVersion(kind)
	if err != nil {
		return errfmt.Errorf("failed to get latest version for %s: %w", kind, err)
	}

	return ig.GenerateInstance(kind, id, version)
}
