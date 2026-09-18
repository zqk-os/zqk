package core

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/zqk-os/zqk/pkg/errfmt"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const emptyValue = ""

// BaseGenerator provides common generator functionality
// Domain-specific generators can embed this for common behavior
type BaseGenerator[S Spec, T any] struct {
	builderFactory BuilderFactory[S, T]
	writer         Writer[T]
	constantsGen   ConstantsGenerator[S]
	outputDir      string
}

// NewBaseGenerator creates a new base generator
func NewBaseGenerator[S Spec, T any](
	builderFactory BuilderFactory[S, T],
	writer Writer[T],
	outputDir string,
) *BaseGenerator[S, T] {
	return &BaseGenerator[S, T]{
		builderFactory: builderFactory,
		writer:         writer,
		outputDir:      outputDir,
	}
}

// NewBaseGeneratorWithConstants creates a new base generator with constants generation support
func NewBaseGeneratorWithConstants[S Spec, T any](
	builderFactory BuilderFactory[S, T],
	writer Writer[T],
	constantsGen ConstantsGenerator[S],
	outputDir string,
) *BaseGenerator[S, T] {
	return &BaseGenerator[S, T]{
		builderFactory: builderFactory,
		writer:         writer,
		constantsGen:   constantsGen,
		outputDir:      outputDir,
	}
}

// GenerateFromSpec generates a single artifact from a spec
func (bg *BaseGenerator[S, T]) GenerateFromSpec(spec S) (T, error) {
	builder := bg.builderFactory.CreateBuilder(spec)
	artifact := builder.Build()

	return artifact, nil
}

// GenerateFromSpecs generates multiple artifacts from specs
func (bg *BaseGenerator[S, T]) GenerateFromSpecs(specs []S) ([]T, error) {
	artifacts := make([]T, len(specs))

	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(8)

	for i, spec := range specs {
		i, spec := i, spec
		g.Go(func() error {
			artifact, err := bg.GenerateFromSpec(spec)
			if err != nil {
				return errfmt.Newf("failed to generate from spec %s", spec.GetName()).Wrap(err)
			}
			artifacts[i] = artifact
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return artifacts, nil
}

// GenerateAndWriteFromSpec generates and writes an artifact from a spec
func (bg *BaseGenerator[S, T]) GenerateAndWriteFromSpec(spec S, filePath string) error {
	artifact, err := bg.GenerateFromSpec(spec)
	if err != nil {
		return err
	}

	if filePath == emptyValue {
		// Generate filename from spec name
		filePath = bg.generateFilename(spec.GetName(), 0)
	}

	fullPath := filepath.Join(bg.outputDir, filePath)
	return bg.writer.WriteToFile(artifact, fullPath)
}

// GenerateAndWriteFromSpecs generates and writes artifacts from specs
func (bg *BaseGenerator[S, T]) GenerateAndWriteFromSpecs(specs []S) error {
	if bg.outputDir != "" {
		if err := fileutil.MkdirAll(bg.outputDir, OutputDirectoryPerm); err != nil {
			return errfmt.Errorf("failed to ensure output directory: %w", err)
		}
	}

	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(8)

	for i, spec := range specs {
		i, spec := i, spec
		g.Go(func() error {
			filename := bg.generateFilename(spec.GetName(), i)
			if err := bg.GenerateAndWriteFromSpec(spec, filename); err != nil {
				return errfmt.Newf("failed to generate and write spec %s", spec.GetName()).Wrap(err)
			}
			return nil
		})
	}
	return g.Wait()
}

// generateFilename generates a filename from a spec name
func (bg *BaseGenerator[S, T]) generateFilename(name string, index int) string {
	// Simple sanitization: replace spaces and dashes with underscores, collapse multiple underscores
	filename := ""
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
	if filename != emptyValue && filename[len(filename)-1] == '_' {
		filename = filename[:len(filename)-1]
	}

	// If filename is empty, use index
	if filename == emptyValue {
		filename = fmt.Sprintf("spec_%d", index)
	}

	return filename + FileExtYAML
}

// SetOutputDir sets the output directory
func (bg *BaseGenerator[S, T]) SetOutputDir(dir string) {
	bg.outputDir = dir
}

// GetOutputDir returns the output directory
func (bg *BaseGenerator[S, T]) GetOutputDir() string {
	return bg.outputDir
}

// SetConstantsGenerator sets the constants generator
func (bg *BaseGenerator[S, T]) SetConstantsGenerator(constantsGen ConstantsGenerator[S]) {
	bg.constantsGen = constantsGen
}

// GetConstantsGenerator returns the constants generator
func (bg *BaseGenerator[S, T]) GetConstantsGenerator() ConstantsGenerator[S] {
	return bg.constantsGen
}

// GenerateConstants generates constants from a spec
func (bg *BaseGenerator[S, T]) GenerateConstants(spec S) (Constants, error) {
	if bg.constantsGen == nil {
		return nil, errfmt.Errorf("constants generator not set")
	}
	return bg.constantsGen.GenerateConstants(spec)
}

// GenerateConstantsFromSpecs generates constants from multiple specs
func (bg *BaseGenerator[S, T]) GenerateConstantsFromSpecs(specs []S) ([]Constants, error) {
	if bg.constantsGen == nil {
		return nil, errfmt.Errorf("constants generator not set")
	}
	return bg.constantsGen.GenerateConstantsFromSpecs(specs)
}

// SanitizeFilename sanitizes a string to be used as a filename
func SanitizeFilename(name string) string {
	// Replace invalid characters
	result := strings.Builder{}
	lastUnderscore := false

	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' {
			result.WriteRune(r)
			lastUnderscore = false
		} else if (r == ' ' || r == '-' || r == '_') && !lastUnderscore {
			result.WriteByte('_')
			lastUnderscore = true
		}
	}

	filename := result.String()

	// Remove trailing underscore
	if filename != emptyValue && filename[len(filename)-1] == '_' {
		filename = filename[:len(filename)-1]
	}

	// Ensure not empty
	if filename == emptyValue {
		filename = "unnamed"
	}

	return filename
}
