package routing_builders

import (
	"context"
	"path/filepath"

	"golang.org/x/sync/errgroup"

	"github.com/zqk-os/zqk/pkg/errfmt"
	sbcore "github.com/zqk-os/zqk/pkg/specbuilder/core"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// RoutingRuleGenerator generates routing rule files from versioned builders
type RoutingRuleGenerator struct {
	registry  *VersionedRoutingRuleBuilderRegistry
	outputDir string
}

// NewRoutingRuleGenerator creates a new routing rule generator
func NewRoutingRuleGenerator(outputDir string) *RoutingRuleGenerator {
	return &RoutingRuleGenerator{
		registry:  GetGlobalRegistry(),
		outputDir: outputDir,
	}
}

// EnsureOutputDir ensures the output directory exists
func (rg *RoutingRuleGenerator) EnsureOutputDir() error {
	return fileutil.MkdirAll(rg.outputDir, sbcore.OutputDirectoryPerm)
}

// GenerateRoutingRules generates a routing rules file for a specific file name and version
func (rg *RoutingRuleGenerator) GenerateRoutingRules(fileName, version string) error {
	builder, err := rg.registry.GetBuilder(fileName, version)
	if err != nil {
		return errfmt.Errorf("failed to get builder for %s@%s: %w", fileName, version, err)
	}

	yamlData, err := builder.Build()
	if err != nil {
		return errfmt.Newf("failed to build routing rules").Wrap(err)
	}

	filename := fileName + sbcore.FileExtYAML
	filePath := filepath.Join(rg.outputDir, filename)

	if err := fileutil.WriteFile(filePath, yamlData, sbcore.OutputFilePerm); err != nil {
		return errfmt.Newf("failed to write routing rules file").Wrap(err)
	}

	return nil
}

// GenerateLatestRoutingRules generates the latest version of routing rules
func (rg *RoutingRuleGenerator) GenerateLatestRoutingRules(fileName string) error {
	version, err := rg.registry.GetLatestVersion(fileName)
	if err != nil {
		return errfmt.Errorf("failed to get latest version for %s: %w", fileName, err)
	}

	return rg.GenerateRoutingRules(fileName, version)
}

// GenerateAllRoutingRules generates all routing rules from all registered builders (latest version only)
func (rg *RoutingRuleGenerator) GenerateAllRoutingRules() error {
	if err := rg.EnsureOutputDir(); err != nil {
		return errfmt.Errorf("failed to ensure output directory: %w", err)
	}

	fileNames := rg.registry.GetAllFileNames()

	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(8)

	for _, fileName := range fileNames {
		fileName := fileName
		g.Go(func() error {
			if err := rg.GenerateLatestRoutingRules(fileName); err != nil {
				return errfmt.Errorf("failed to generate routing rules for %s: %w", fileName, err)
			}
			return nil
		})
	}

	return g.Wait()
}
