package trait_builders

import (
	"context"
	"path/filepath"

	"golang.org/x/sync/errgroup"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	sbcore "github.com/lanceman/zqk/pkg/specbuilder/core"
	sbyaml "github.com/lanceman/zqk/pkg/specbuilder/yaml"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TraitGenerator generates trait files from versioned builders
type TraitGenerator struct {
	registry  *VersionedTraitBuilderRegistry
	outputDir string
	writer    *sbyaml.YAMLWriter[*objects.TraitDefinition]
}

// NewTraitGenerator creates a new trait generator
func NewTraitGenerator(outputDir string) *TraitGenerator {
	return &TraitGenerator{
		registry:  GetGlobalRegistry(),
		outputDir: outputDir,
		writer:    sbyaml.NewYAMLWriter[*objects.TraitDefinition](),
	}
}

// EnsureOutputDir ensures the output directory exists
func (tg *TraitGenerator) EnsureOutputDir() error {
	return fileutil.MkdirAll(tg.outputDir, sbcore.OutputDirectoryPerm)
}

// GenerateTrait generates a trait file for a specific name and version
func (tg *TraitGenerator) GenerateTrait(name, version string) error {
	builder, err := tg.registry.GetBuilder(name, version)
	if err != nil {
		return errfmt.Errorf("failed to get builder for %s@%s: %w", name, version, err)
	}

	trait := builder.Build()
	filename := name + sbcore.FileExtYAML
	filePath := filepath.Join(tg.outputDir, filename)

	if err := tg.writer.WriteToFile(trait, filePath); err != nil {
		return errfmt.Newf("failed to write trait file").Wrap(err)
	}

	return nil
}

// GenerateLatestTrait generates the latest version of a trait
func (tg *TraitGenerator) GenerateLatestTrait(name string) error {
	version, err := tg.registry.GetLatestVersion(name)
	if err != nil {
		return errfmt.Errorf("failed to get latest version for %s: %w", name, err)
	}

	return tg.GenerateTrait(name, version)
}

// GenerateAllTraits generates all traits from all registered builders (latest version only)
func (tg *TraitGenerator) GenerateAllTraits() error {
	if err := tg.EnsureOutputDir(); err != nil {
		return errfmt.Errorf("failed to ensure output directory: %w", err)
	}

	names := tg.registry.GetAllNames()

	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(8)

	for _, name := range names {
		name := name
		g.Go(func() error {
			if err := tg.GenerateLatestTrait(name); err != nil {
				return errfmt.Errorf("failed to generate trait for %s: %w", name, err)
			}
			return nil
		})
	}

	return g.Wait()
}
