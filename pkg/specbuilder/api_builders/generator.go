package api_builders

import (
	"context"
	"path/filepath"

	"golang.org/x/sync/errgroup"

	"github.com/lanceman/zqk/pkg/errfmt"
	sbcore "github.com/lanceman/zqk/pkg/specbuilder/core"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// APIGenerator generates api files from versioned builders
type APIGenerator struct {
	registry  *VersionedAPISpecGenBuilderRegistry
	outputDir string
}

// NewAPIGenerator creates a new api generator
func NewAPIGenerator(outputDir string) *APIGenerator {
	return &APIGenerator{
		registry:  GetGlobalRegistry(),
		outputDir: outputDir,
	}
}

// EnsureOutputDir ensures the output directory exists
func (cg *APIGenerator) EnsureOutputDir() error {
	return fileutil.MkdirAll(cg.outputDir, sbcore.OutputDirectoryPerm)
}

// GenerateAPI generates a api file for a specific file name and version
func (cg *APIGenerator) GenerateAPI(fileName, version string) error {
	builder, err := cg.registry.GetBuilder(fileName, version)
	if err != nil {
		return errfmt.Errorf("failed to get builder for %s@%s: %w", fileName, version, err)
	}

	yamlData, err := builder.Build()
	if err != nil {
		return errfmt.Newf("failed to build api").Wrap(err)
	}

	filename := fileName + sbcore.FileExtYAML
	filePath := filepath.Join(cg.outputDir, filename)

	if err := fileutil.WriteFile(filePath, yamlData, sbcore.OutputFilePerm); err != nil {
		return errfmt.Newf("failed to write api file").Wrap(err)
	}

	return nil
}

// GenerateLatestAPI generates the latest version of a api
func (cg *APIGenerator) GenerateLatestAPI(fileName string) error {
	version, err := cg.registry.GetLatestVersion(fileName)
	if err != nil {
		return errfmt.Errorf("failed to get latest version for %s: %w", fileName, err)
	}

	return cg.GenerateAPI(fileName, version)
}

// GenerateAllAPIs generates all apis from all registered builders (latest version only)
func (cg *APIGenerator) GenerateAllAPIs() error {
	if err := cg.EnsureOutputDir(); err != nil {
		return errfmt.Errorf("failed to ensure output directory: %w", err)
	}

	fileNames := cg.registry.GetAllFileNames()

	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(8)

	for _, fileName := range fileNames {
		fileName := fileName
		g.Go(func() error {
			if err := cg.GenerateLatestAPI(fileName); err != nil {
				return errfmt.Errorf("failed to generate api for %s: %w", fileName, err)
			}
			return nil
		})
	}

	return g.Wait()
}
