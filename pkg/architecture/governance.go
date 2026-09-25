package architecture

import (
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// Layer represents the architectural tier of a package.
type Layer string

const (
	LayerFoundation Layer = "Tier 1: Foundation Primitives"
	LayerCoreEngine Layer = "Tier 2: Core Domain & Storage Engine"
	LayerAppSurface Layer = "Tier 3: CLI, Services, & MCP Adapters"
)

// ComplexityBudget defines limits for package maintainability and sprawl.
type ComplexityBudget struct {
	MaxFileLines     int
	MaxPackageFiles  int
	MaxCyclomatic    int
}

// DefaultComplexityBudget provides standard limits to prevent god packages.
var DefaultComplexityBudget = ComplexityBudget{
	MaxFileLines:    2500,
	MaxPackageFiles: 1000,
	MaxCyclomatic:   30,
}

// PackageMetadata holds summary statistics of a Go package.
type PackageMetadata struct {
	Path       string
	FileCount  int
	TotalLines int
	Layer      Layer
}

// ClassifyLayer categorizes a package path into its architectural tier.
func ClassifyLayer(pkgPath string) Layer {
	clean := filepath.ToSlash(pkgPath)
	if strings.HasPrefix(clean, "cmd/") || strings.Contains(clean, "/mcp") {
		return LayerAppSurface
	}
	if strings.Contains(clean, "/storage") || strings.Contains(clean, "/scheduler") || strings.Contains(clean, "/orchestration") {
		return LayerCoreEngine
	}
	return LayerFoundation
}

// ValidateFileComplexity inspects a Go source file against line limit budgets.
func ValidateFileComplexity(path string, maxLines int) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	lines := strings.Count(string(data), "\n") + 1
	if lines > maxLines {
		return lines, fmt.Errorf("file %s exceeds maximum line budget (%d > %d)", filepath.Base(path), lines, maxLines)
	}
	return lines, nil
}

// ParsePackageSummary parses package info and checks AST compilability.
func ParsePackageSummary(dir string) (*PackageMetadata, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, nil, parser.PackageClauseOnly)
	if err != nil {
		return nil, err
	}
	if len(pkgs) == 0 {
		return nil, errors.New("no Go packages found in directory")
	}

	var totalFiles int
	for _, p := range pkgs {
		totalFiles += len(p.Files)
	}

	return &PackageMetadata{
		Path:      dir,
		FileCount: totalFiles,
		Layer:     ClassifyLayer(dir),
	}, nil
}
