package testing

import (
	"fmt"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/errfmt"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

const emptyValue = ""

// ScenarioSpec represents a scenario specification that can be used to generate a scenario
// This is essentially the same as TestScenario, but used as input for generation
type ScenarioSpec struct {
	Name              string     `yaml:"name"`
	Description       string     `yaml:"description,omitempty"`
	ResponseProcessor string     `yaml:"response_processor,omitempty"`
	Imports           []string   `yaml:"imports,omitempty"`
	Setup             []TestStep `yaml:"setup,omitempty"`
	Tests             []TestStep `yaml:"tests"`
	Cleanup           []TestStep `yaml:"cleanup,omitempty"`
	Variables         any        `yaml:"variables,omitempty"`
}

// ScenarioSpecList represents a collection of scenario specifications
type ScenarioSpecList struct {
	Scenarios []ScenarioSpec `yaml:"scenarios"`
}

// ScenarioGenerator generates test scenarios from YAML specifications using the builder API
//
// Deprecated: This implementation is being migrated to use the specbuilder infrastructure.
// New code should use github.com/lanceman/zqk/pkg/specbuilder/generators.ScenarioGenerator instead.
// This implementation remains available for backward compatibility during migration.
// See pkg/specbuilder/MIGRATION_GUIDE.md for migration instructions.
type ScenarioGenerator struct {
	outputDir string
}

// NewScenarioGenerator creates a new scenario generator
//
// Deprecated: Use github.com/lanceman/zqk/pkg/specbuilder/generators.NewScenarioGenerator instead.
// See pkg/specbuilder/MIGRATION_GUIDE.md for migration instructions.
func NewScenarioGenerator(outputDir string) *ScenarioGenerator {
	return &ScenarioGenerator{
		outputDir: outputDir,
	}
}

// GenerateFromFile reads a YAML file containing scenario specifications and generates scenarios
func (sg *ScenarioGenerator) GenerateFromFile(specFile string) error {
	data, err := fileutil.ReadFile(specFile)
	if err != nil {
		return errfmt.Newf("failed to read spec file").Wrap(err)
	}

	var specList ScenarioSpecList
	if err := yaml.Unmarshal(data, &specList); err != nil {
		return errfmt.Newf("failed to parse spec file").Wrap(err)
	}

	return sg.GenerateFromSpecs(specList.Scenarios)
}

// GenerateFromSpecs generates scenarios from a list of specifications
func (sg *ScenarioGenerator) GenerateFromSpecs(specs []ScenarioSpec) error {
	writer := NewScenarioWriter()

	for i, spec := range specs {
		scenario := sg.buildScenarioFromSpec(spec)

		// Generate filename from scenario name (sanitize)
		filename := sg.generateFilename(spec.Name, i)
		outputPath := filepath.Join(sg.outputDir, filename)

		if err := writer.WriteToFile(scenario, outputPath); err != nil {
			return errfmt.Errorf("failed to write scenario %s: %w", spec.Name, err)
		}
	}

	return nil
}

// buildScenarioFromSpec builds a TestScenario from a ScenarioSpec using the builder API
func (sg *ScenarioGenerator) buildScenarioFromSpec(spec ScenarioSpec) *TestScenario {
	builder := NewScenarioBuilder().
		Name(spec.Name)

	if spec.Description != emptyValue {
		builder.Description(spec.Description)
	}

	if spec.ResponseProcessor != emptyValue {
		builder.ResponseProcessor(spec.ResponseProcessor)
	}

	// Add imports
	for _, importPath := range spec.Imports {
		builder.AddImport(importPath)
	}

	// Add setup steps
	for _, step := range spec.Setup {
		stepCopy := step // Copy to avoid pointer issues
		builder.AddSetupStep(&stepCopy)
	}

	// Add test steps
	for _, step := range spec.Tests {
		stepCopy := step // Copy to avoid pointer issues
		builder.AddTestStep(&stepCopy)
	}

	// Add cleanup steps
	for _, step := range spec.Cleanup {
		stepCopy := step // Copy to avoid pointer issues
		builder.AddCleanupStep(&stepCopy)
	}

	return builder.Build()
}

// generateFilename generates a filename from a scenario name
func (sg *ScenarioGenerator) generateFilename(name string, index int) string {
	// Simple sanitization: replace spaces and dashes with underscores, collapse multiple underscores
	filename := emptyValue
	lastUnderscore := false
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			filename += string(r)
			lastUnderscore = false
		} else if (r == ' ' || r == '-') && !lastUnderscore {
			filename += "_"
			lastUnderscore = true
		}
	}

	// Remove trailing underscore if present
	if len(filename) > 0 && filename[len(filename)-1] == '_' {
		filename = filename[:len(filename)-1]
	}

	// If filename is empty, use index
	if filename == emptyValue {
		filename = fmt.Sprintf("scenario_%d", index)
	}

	return filename + ".yaml"
}

// GenerateScenariosFromSpecFile is a convenience function that generates scenarios from a spec file
//
// Deprecated: Use github.com/lanceman/zqk/pkg/specbuilder/generators.GenerateScenariosFromSpecFile instead.
// See pkg/specbuilder/MIGRATION_GUIDE.md for migration instructions.
func GenerateScenariosFromSpecFile(specFile, outputDir string) error {
	generator := NewScenarioGenerator(outputDir)
	return generator.GenerateFromFile(specFile)
}
