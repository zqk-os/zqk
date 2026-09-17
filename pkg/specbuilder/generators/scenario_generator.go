package generators

import (
	"context"
	"path/filepath"

	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/errfmt"
	mcptesting "github.com/lanceman/zqk/pkg/mcp/testing"
	"github.com/lanceman/zqk/pkg/specbuilder/adapters"
	sbcore "github.com/lanceman/zqk/pkg/specbuilder/core"
	sbyaml "github.com/lanceman/zqk/pkg/specbuilder/yaml"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

const scenarioGeneratorFileExtYAML = ".yaml"

// ScenarioGenerator uses specbuilder core to generate test scenarios
// This is the new implementation that uses the specbuilder infrastructure
// It can coexist with the existing testing.ScenarioGenerator
type ScenarioGenerator struct {
	outputDir string
	factory   *adapters.ScenarioBuilderFactoryAdapter
	writer    *sbyaml.YAMLWriter[*mcptesting.TestScenario]
}

// NewScenarioGenerator creates a new scenario generator using specbuilder infrastructure
func NewScenarioGenerator(outputDir string) *ScenarioGenerator {
	return &ScenarioGenerator{
		outputDir: outputDir,
		factory:   adapters.NewScenarioBuilderFactoryAdapter(),
		writer:    sbyaml.NewYAMLWriter[*mcptesting.TestScenario](),
	}
}

// GenerateFromFile loads specs from a YAML file and generates scenarios
// This is a convenience method that uses the specbuilder infrastructure
func (sg *ScenarioGenerator) GenerateFromFile(filePath string) error {
	// Load YAML file manually (since ScenarioSpec doesn't implement core.Spec)
	data, err := fileutil.ReadFile(filePath)
	if err != nil {
		return errfmt.Newf("failed to read spec file").Wrap(err)
	}

	var wrapper struct {
		Scenarios []mcptesting.ScenarioSpec `yaml:"scenarios"`
	}
	if err := yaml.Unmarshal(data, &wrapper); err != nil {
		return errfmt.Newf("failed to parse YAML").Wrap(err)
	}

	// Convert to adapters and generate
	specAdapters := make([]*adapters.ScenarioSpecAdapter, len(wrapper.Scenarios))
	for i := range wrapper.Scenarios {
		specAdapters[i] = adapters.NewScenarioSpecAdapter(wrapper.Scenarios[i])
	}

	return sg.generateAndWrite(specAdapters)
}

// GenerateFromSpecs generates scenarios from a list of specs
// This is compatible with the existing testing.ScenarioGenerator interface
func (sg *ScenarioGenerator) GenerateFromSpecs(specs []mcptesting.ScenarioSpec) error {
	// Convert specs to adapters
	specAdapters := make([]*adapters.ScenarioSpecAdapter, len(specs))
	for i := range specs {
		specAdapters[i] = adapters.NewScenarioSpecAdapter(specs[i])
	}

	return sg.generateAndWrite(specAdapters)
}

// generateAndWrite generates and writes scenarios from adapters
func (sg *ScenarioGenerator) generateAndWrite(specAdapters []*adapters.ScenarioSpecAdapter) error {
	if err := fileutil.MkdirAll(sg.outputDir, sbcore.OutputDirectoryPerm); err != nil {
		return errfmt.Errorf("failed to ensure output directory: %w", err)
	}

	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(8)

	for _, specAdapter := range specAdapters {
		specAdapter := specAdapter
		g.Go(func() error {
			// Create builder from adapter
			builder := sg.factory.CreateBuilder(specAdapter)
			artifact := builder.Build()

			// Generate filename
			filename := sbcore.SanitizeFilename(specAdapter.GetName()) + scenarioGeneratorFileExtYAML

			// Write to file
			filePath := filepath.Join(sg.outputDir, filename)
			if err := sg.writer.WriteToFile(artifact, filePath); err != nil {
				return errfmt.Newf("failed to write scenario %s", specAdapter.GetName()).Wrap(err)
			}
			return nil
		})
	}
	return g.Wait()
}

// GenerateScenariosFromSpecFile is a convenience function that creates a generator and generates scenarios
func GenerateScenariosFromSpecFile(specFile, outputDir string) error {
	generator := NewScenarioGenerator(outputDir)
	return generator.GenerateFromFile(specFile)
}
