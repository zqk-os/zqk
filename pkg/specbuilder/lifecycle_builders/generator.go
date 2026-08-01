package lifecycle_builders

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	sbyaml "github.com/lanceman/zqk/pkg/specbuilder/yaml"
	"golang.org/x/sync/errgroup"
)

// LifecycleGenerator generates lifecycle files from versioned builders
type LifecycleGenerator struct {
	registry  *VersionedLifecycleBuilderRegistry
	outputDir string
	writer    *sbyaml.YAMLWriter[*objects.Lifecycle]
}

// NewLifecycleGenerator creates a new lifecycle generator
func NewLifecycleGenerator(outputDir string) *LifecycleGenerator {
	return &LifecycleGenerator{
		registry:  GetGlobalRegistry(),
		outputDir: outputDir,
		writer:    sbyaml.NewYAMLWriter[*objects.Lifecycle](),
	}
}

// EnsureOutputDir ensures the output directory exists
func (lg *LifecycleGenerator) EnsureOutputDir() error {
	return os.MkdirAll(lg.outputDir, paths.DirPerm755)
}

// GenerateLifecycle generates a lifecycle file for a specific object type and version
func (lg *LifecycleGenerator) GenerateLifecycle(objectType, version string) error {
	builder, err := lg.registry.GetBuilder(objectType, version)
	if err != nil {
		return errfmt.Errorf("failed to get builder for %s@%s: %w", objectType, version, err)
	}

	lifecycle := builder.Build()
	filename := fmt.Sprintf("%s_lifecycle.yaml", objectType)
	filePath := filepath.Join(lg.outputDir, filename)

	if err := lg.writer.WriteToFile(lifecycle, filePath); err != nil {
		return errfmt.Newf("failed to write lifecycle file").Wrap(err)
	}

	return nil
}

// GenerateLatestLifecycle generates the latest version of a lifecycle
func (lg *LifecycleGenerator) GenerateLatestLifecycle(objectType string) error {
	version, err := lg.registry.GetLatestVersion(objectType)
	if err != nil {
		return errfmt.Errorf("failed to get latest version for %s: %w", objectType, err)
	}

	return lg.GenerateLifecycle(objectType, version)
}

// GenerateAllLifecycles generates all lifecycles from all registered builders (latest version only)
func (lg *LifecycleGenerator) GenerateAllLifecycles() error {
	if err := lg.EnsureOutputDir(); err != nil {
		return errfmt.Errorf("failed to ensure output directory: %w", err)
	}

	objectTypes := lg.registry.GetAllObjectTypes()

	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(8)

	for _, objectType := range objectTypes {
		objectType := objectType
		g.Go(func() error {
			if err := lg.GenerateLatestLifecycle(objectType); err != nil {
				return errfmt.Errorf("failed to generate lifecycle for %s: %w", objectType, err)
			}
			return nil
		})
	}

	return g.Wait()
}
