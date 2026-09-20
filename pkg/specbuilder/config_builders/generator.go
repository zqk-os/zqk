package config_builders

import (
	"context"
	"path/filepath"

	"golang.org/x/sync/errgroup"

	"github.com/zqk-os/zqk/pkg/errfmt"
	sbcore "github.com/zqk-os/zqk/pkg/specbuilder/core"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// ConfigGenerator generates config files from versioned builders
type ConfigGenerator struct {
	registry  *VersionedConfigBuilderRegistry
	outputDir string
}

// NewConfigGenerator creates a new config generator
func NewConfigGenerator(outputDir string) *ConfigGenerator {
	return &ConfigGenerator{
		registry:  GetGlobalRegistry(),
		outputDir: outputDir,
	}
}

// EnsureOutputDir ensures the output directory exists
func (cg *ConfigGenerator) EnsureOutputDir() error {
	return fileutil.MkdirAll(cg.outputDir, sbcore.OutputDirectoryPerm)
}

// GenerateConfig generates a config file for a specific file name and version
func (cg *ConfigGenerator) GenerateConfig(fileName, version string) error {
	builder, err := cg.registry.GetBuilder(fileName, version)
	if err != nil {
		return errfmt.Errorf("failed to get builder for %s@%s: %w", fileName, version, err)
	}

	yamlData, err := builder.Build()
	if err != nil {
		return errfmt.Newf("failed to build config").Wrap(err)
	}

	filename := fileName + sbcore.FileExtYAML
	filePath := filepath.Join(cg.outputDir, filename)

	if err := fileutil.WriteFile(filePath, yamlData, sbcore.OutputFilePerm); err != nil {
		return errfmt.Newf("failed to write config file").Wrap(err)
	}

	return nil
}

// GenerateLatestConfig generates the latest version of a config
func (cg *ConfigGenerator) GenerateLatestConfig(fileName string) error {
	version, err := cg.registry.GetLatestVersion(fileName)
	if err != nil {
		return errfmt.Errorf("failed to get latest version for %s: %w", fileName, err)
	}

	return cg.GenerateConfig(fileName, version)
}

// GenerateAllConfigs generates all configs from all registered builders (latest version only)
func (cg *ConfigGenerator) GenerateAllConfigs() error {
	if err := cg.EnsureOutputDir(); err != nil {
		return errfmt.Errorf("failed to ensure output directory: %w", err)
	}

	fileNames := cg.registry.GetAllFileNames()

	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(8)

	for _, fileName := range fileNames {
		fileName := fileName
		g.Go(func() error {
			if err := cg.GenerateLatestConfig(fileName); err != nil {
				return errfmt.Errorf("failed to generate config for %s: %w", fileName, err)
			}
			return nil
		})
	}

	return g.Wait()
}
