package profile_builders

import (
	"context"
	"path/filepath"

	"golang.org/x/sync/errgroup"

	"github.com/lanceman/zqk/pkg/config"
	"github.com/lanceman/zqk/pkg/errfmt"
	sbcore "github.com/lanceman/zqk/pkg/specbuilder/core"
	sbyaml "github.com/lanceman/zqk/pkg/specbuilder/yaml"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// ProfileGenerator generates profile files from versioned builders
type ProfileGenerator struct {
	registry  *VersionedProfileBuilderRegistry
	outputDir string
	writer    *sbyaml.YAMLWriter[*config.UnifiedProfile]
}

// NewProfileGenerator creates a new profile generator
func NewProfileGenerator(outputDir string) *ProfileGenerator {
	return &ProfileGenerator{
		registry:  GetGlobalRegistry(),
		outputDir: outputDir,
		writer:    sbyaml.NewYAMLWriter[*config.UnifiedProfile](),
	}
}

// EnsureOutputDir ensures the output directory exists
func (pg *ProfileGenerator) EnsureOutputDir() error {
	return fileutil.MkdirAll(pg.outputDir, sbcore.OutputDirectoryPerm)
}

// GenerateProfile generates a profile file for a specific name and version
func (pg *ProfileGenerator) GenerateProfile(name, version string) error {
	builder, err := pg.registry.GetBuilder(name, version)
	if err != nil {
		return errfmt.Errorf("failed to get builder for %s@%s: %w", name, version, err)
	}

	profile := builder.Build()
	filename := name + sbcore.FileExtYAML
	filePath := filepath.Join(pg.outputDir, filename)

	if err := pg.writer.WriteToFile(profile, filePath); err != nil {
		return errfmt.Newf("failed to write profile file").Wrap(err)
	}

	return nil
}

// GenerateLatestProfile generates the latest version of a profile
func (pg *ProfileGenerator) GenerateLatestProfile(name string) error {
	version, err := pg.registry.GetLatestVersion(name)
	if err != nil {
		return errfmt.Errorf("failed to get latest version for %s: %w", name, err)
	}

	return pg.GenerateProfile(name, version)
}

// GenerateAllProfiles generates all profiles from all registered builders (latest version only)
func (pg *ProfileGenerator) GenerateAllProfiles() error {
	if err := pg.EnsureOutputDir(); err != nil {
		return errfmt.Errorf("failed to ensure output directory: %w", err)
	}

	names := pg.registry.GetAllNames()

	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(8)

	for _, name := range names {
		name := name
		g.Go(func() error {
			if err := pg.GenerateLatestProfile(name); err != nil {
				return errfmt.Errorf("failed to generate profile for %s: %w", name, err)
			}
			return nil
		})
	}

	return g.Wait()
}
