package objects

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/appledouble"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"gopkg.in/yaml.v3"
)

// SpecDependencyGraph represents the dependency graph of object specifications
type SpecDependencyGraph struct {
	loader *SpecLoader
	specs  map[string]*SpecInfo // ontology -> spec info
	edges  map[string][]string  // ontology -> list of dependent ontologies
	// builtAtSpecCacheRevision is SpecLoader.SpecCacheRevision() after the last successful BuildGraph.
	builtAtSpecCacheRevision uint64
}

// SpecInfo contains minimal info about a spec for dependency analysis
type SpecInfo struct {
	Ontology string
	Extends  string
	FilePath string
	Spec     *Spec // Full spec (loaded when needed)
}

// GraphValidationError represents a validation error in the dependency graph
type GraphValidationError struct {
	Type    string // "cycle", "missing_parent", "invalid_extends"
	Message string
	Specs   []string // Affected specs
}

func (e *GraphValidationError) Error() string {
	return fmt.Sprintf("%s: %s (specs: %v)", e.Type, e.Message, e.Specs)
}

// LogFields returns structured fields for logging
func (e *GraphValidationError) LogFields() []any {
	return []any{
		"type", e.Type,
		"message", e.Message,
		"specs", e.Specs,
	}
}

// NewSpecDependencyGraph creates a new dependency graph
func NewSpecDependencyGraph(loader *SpecLoader) *SpecDependencyGraph {
	return &SpecDependencyGraph{
		loader: loader,
		specs:  make(map[string]*SpecInfo),
		edges:  make(map[string][]string),
	}
}

// BuildGraph scans all spec files and builds the dependency graph
func (sdg *SpecDependencyGraph) BuildGraph() error {
	if sdg.loader == nil {
		return errfmt.Errorf("spec loader not provided")
	}

	// Scan spec directory
	entries, err := os.ReadDir(sdg.loader.specsDir)
	if err != nil {
		return errfmt.Newf("failed to read spec directory").Wrap(err)
	}

	// First pass: collect all specs
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		// Skip macOS AppleDouble/resource-fork files (._*)
		if appledouble.SkipNameInReadDir(entry.Name()) {
			continue
		}

		specPath := filepath.Join(sdg.loader.specsDir, entry.Name())
		info, err := sdg.parseSpecInfo(specPath)
		if err != nil {
			// Log warning but skip files that can't be parsed
			eventLogger := logging.NewEventLogger(pkgctx.NewSystemContext())
			eventLogger.LogWarning("Failed to parse spec info",
				logging.String("file", specPath),
				logging.Error(err))
			continue
		}

		if info.Ontology != emptyValue {
			sdg.specs[info.Ontology] = info
		}
	}

	// Second pass: build edges (dependencies)
	for ontology, info := range sdg.specs {
		if info.Extends != emptyValue && info.Extends != "null" {
			// Add edge: extends -> ontology (parent -> child)
			// This means parent must be loaded before child
			if _, exists := sdg.edges[info.Extends]; !exists {
				sdg.edges[info.Extends] = []string{}
			}
			sdg.edges[info.Extends] = append(sdg.edges[info.Extends], ontology)
		}
	}

	sdg.builtAtSpecCacheRevision = sdg.loader.SpecCacheRevision()
	return nil
}

// BuiltAtSpecCacheRevision returns SpecLoader.SpecCacheRevision() captured after the last successful
// [SpecDependencyGraph.BuildGraph]. Compare with [SpecLoader.SpecCacheRevision] to detect
// invalidation since the graph was built.
func (sdg *SpecDependencyGraph) BuiltAtSpecCacheRevision() uint64 {
	if sdg == nil {
		return 0
	}
	return sdg.builtAtSpecCacheRevision
}

// LoaderSpecCacheInvalidatedSinceBuild reports whether the associated [SpecLoader] has been
// cleared or invalidated since the last successful [SpecDependencyGraph.BuildGraph].
func (sdg *SpecDependencyGraph) LoaderSpecCacheInvalidatedSinceBuild() bool {
	if sdg == nil || sdg.loader == nil {
		return true
	}
	return sdg.loader.SpecCacheRevision() != sdg.builtAtSpecCacheRevision
}

// parseSpecInfo parses minimal info from a spec file without full loading
func (sdg *SpecDependencyGraph) parseSpecInfo(filePath string) (*SpecInfo, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var partial struct {
		Ontology string `yaml:"ontology"`
		Extends  string `yaml:"extends"`
	}

	if err := yaml.Unmarshal(data, &partial); err != nil {
		return nil, err
	}

	return &SpecInfo{
		Ontology: partial.Ontology,
		Extends:  partial.Extends,
		FilePath: filePath,
	}, nil
}

// DetectCycles detects circular dependencies in the graph
func (sdg *SpecDependencyGraph) DetectCycles() []*GraphValidationError {
	var errors []*GraphValidationError

	// Use DFS to detect cycles
	visited := make(map[string]bool)
	recStack := make(map[string]bool)
	cyclePath := []string{}

	var dfs func(ontology string) bool
	dfs = func(ontology string) bool {
		visited[ontology] = true
		recStack[ontology] = true
		cyclePath = append(cyclePath, ontology)

		// Check all dependents
		for _, dependent := range sdg.edges[ontology] {
			if !visited[dependent] {
				if dfs(dependent) {
					return true
				}
			} else if recStack[dependent] {
				// Found a cycle
				cycleStart := -1
				for i, spec := range cyclePath {
					if spec == dependent {
						cycleStart = i
						break
					}
				}
				if cycleStart >= 0 {
					//nolint:gocritic // Intentionally creating new slice from slice segment
					cycle := append(cyclePath[cycleStart:], dependent)
					errors = append(errors, &GraphValidationError{
						Type:    "cycle",
						Message: "Circular dependency detected",
						Specs:   cycle,
					})
				}
				return true
			}
		}

		recStack[ontology] = false
		cyclePath = cyclePath[:len(cyclePath)-1]
		return false
	}

	// Check all nodes
	for ontology := range sdg.specs {
		if !visited[ontology] {
			cyclePath = []string{}
			dfs(ontology)
		}
	}

	return errors
}

// FindMissingParents finds specs that extend non-existent parents
func (sdg *SpecDependencyGraph) FindMissingParents() []*GraphValidationError {
	var errors []*GraphValidationError

	for ontology, info := range sdg.specs {
		if info.Extends != emptyValue && info.Extends != "null" {
			if _, exists := sdg.specs[info.Extends]; !exists {
				errors = append(errors, &GraphValidationError{
					Type:    "missing_parent",
					Message: fmt.Sprintf("Spec '%s' extends non-existent spec '%s'", ontology, info.Extends),
					Specs:   []string{ontology, info.Extends},
				})
			}
		}
	}

	return errors
}

// TopologicalSort performs topological sort on the dependency graph
// Returns specs in load order (parents before children)
func (sdg *SpecDependencyGraph) TopologicalSort() ([]*Spec, error) {
	// First check for cycles
	cycles := sdg.DetectCycles()
	if len(cycles) > 0 {
		return nil, errfmt.Errorf("cannot perform topological sort: cycles detected: %v", cycles)
	}

	// Calculate in-degrees (number of dependencies)
	inDegree := make(map[string]int)
	for ontology := range sdg.specs {
		inDegree[ontology] = 0
	}

	// Count in-degrees
	for ontology, info := range sdg.specs {
		if info.Extends != emptyValue && info.Extends != "null" {
			inDegree[ontology]++
		}
	}

	// Kahn's algorithm for topological sort
	var queue []string
	var result []*Spec

	// Start with nodes that have no dependencies (in-degree = 0)
	for ontology, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, ontology)
		}
	}

	for len(queue) > 0 {
		// Dequeue
		current := queue[0]
		queue = queue[1:]

		// Load the spec
		info := sdg.specs[current]
		spec, err := sdg.loader.LoadSpecWithInheritance(filepath.Base(info.FilePath))
		if err != nil {
			return nil, errfmt.Errorf("failed to load spec %s: %w", current, err)
		}
		result = append(result, spec)

		// Process dependents
		for _, dependent := range sdg.edges[current] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	// Check if all specs were processed
	if len(result) != len(sdg.specs) {
		return nil, errfmt.Errorf("topological sort incomplete: processed %d of %d specs", len(result), len(sdg.specs))
	}

	return result, nil
}

// ValidateGraph performs full validation of the dependency graph
func (sdg *SpecDependencyGraph) ValidateGraph() []*GraphValidationError {
	var errors []*GraphValidationError

	// Check for cycles
	cycles := sdg.DetectCycles()
	errors = append(errors, cycles...)

	// Check for missing parents
	missing := sdg.FindMissingParents()
	errors = append(errors, missing...)

	return errors
}
