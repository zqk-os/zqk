package builders

import (
	"context"
	"path/filepath"

	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	sbcore "github.com/lanceman/zqk/pkg/specbuilder/core"
	sbyaml "github.com/lanceman/zqk/pkg/specbuilder/yaml"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// SpecGenerator generates spec files from versioned builders
type SpecGenerator struct {
	registry  *VersionedBuilderRegistry
	outputDir string
	writer    *sbyaml.YAMLWriter[*objects.Spec]
}

// NewSpecGenerator creates a new spec generator
func NewSpecGenerator(outputDir string) *SpecGenerator {
	return &SpecGenerator{
		registry:  GetGlobalRegistry(),
		outputDir: outputDir,
		writer:    sbyaml.NewYAMLWriter[*objects.Spec](),
	}
}

// GenerateSpec generates a spec file for a specific ontology and version
func (sg *SpecGenerator) GenerateSpec(ontology, version string) error {
	builder, err := sg.registry.GetBuilder(ontology, version)
	if err != nil {
		return errfmt.Errorf("failed to get builder for %s@%s: %w", ontology, version, err)
	}

	spec := builder.Build()
	filename := ontology + sbcore.FileExtYAML
	filePath := filepath.Join(sg.outputDir, filename)

	if err := sg.writer.WriteToFile(spec, filePath); err != nil {
		return errfmt.Newf("failed to write spec file").Wrap(err)
	}

	return nil
}

// GenerateLatestSpec generates the latest version of a spec
func (sg *SpecGenerator) GenerateLatestSpec(ontology string) error {
	version, err := sg.registry.GetLatestVersion(ontology)
	if err != nil {
		return errfmt.Errorf("failed to get latest version for %s: %w", ontology, err)
	}

	return sg.GenerateSpec(ontology, version)
}

// GenerateAllSpecs generates all specs from all registered builders (latest version only)
func (sg *SpecGenerator) GenerateAllSpecs() error {
	if err := sg.EnsureOutputDir(); err != nil {
		return errfmt.Errorf("failed to ensure output directory: %w", err)
	}

	ontologies := sg.registry.GetAllOntologies()

	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(8)

	for _, ontology := range ontologies {
		ontology := ontology
		g.Go(func() error {
			if err := sg.GenerateLatestSpec(ontology); err != nil {
				return errfmt.Errorf("failed to generate spec for %s: %w", ontology, err)
			}
			return nil
		})
	}

	return g.Wait()
}

// GenerateSpecAtVersion generates a spec at a specific version (for version comparison/testing)
func (sg *SpecGenerator) GenerateSpecAtVersion(ontology, version string, outputPath string) error {
	builder, err := sg.registry.GetBuilder(ontology, version)
	if err != nil {
		return errfmt.Errorf("failed to get builder for %s@%s: %w", ontology, version, err)
	}

	spec := builder.Build()

	// Write to specified path (can include version in filename)
	if err := sg.writer.WriteToFile(spec, outputPath); err != nil {
		return errfmt.Newf("failed to write spec file").Wrap(err)
	}

	return nil
}

// GenerateSpecsToYAML generates specs and returns them as YAML bytes (for testing/comparison)
func (sg *SpecGenerator) GenerateSpecsToYAML(ontology, version string) ([]byte, error) {
	builder, err := sg.registry.GetBuilder(ontology, version)
	if err != nil {
		return nil, errfmt.Errorf("failed to get builder for %s@%s: %w", ontology, version, err)
	}

	spec := builder.Build()

	// Marshal to YAML
	data, err := yaml.Marshal(spec)
	if err != nil {
		return nil, errfmt.Newf("failed to marshal spec to YAML").Wrap(err)
	}

	return data, nil
}

// EnsureOutputDir ensures the output directory exists
func (sg *SpecGenerator) EnsureOutputDir() error {
	return fileutil.MkdirAll(sg.outputDir, sbcore.OutputDirectoryPerm)
}
