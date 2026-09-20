package objects

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/appledouble"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// SpecDependencyGraph represents the dependency graph of object specifications
type SpecDependencyGraph struct {
	loader *SpecLoader
	specs  map[string]*SpecInfo // ontology -> spec info
	edges  map[string][]string  // ontology -> list of dependent ontologies
	// builtAtSpecCacheRevision is SpecLoader.SpecCacheRevision() after the last successful BuildGraph.
	builtAtSpecCacheRevision uint64
}

// SpecInfo contains minimal information needed for dependency analysis
type SpecInfo struct {
	Ontology string
	Extends  string
	Composes []string
	FilePath string
	Spec     *Spec // Full spec (loaded when needed)
}

// GraphValidationError represents a validation error in the dependency graph
type GraphValidationError struct {
	Type    string   // "missing_parent", "circular_dependency", "duplicate_spec"
	Message string   // Human-readable error message
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

	// First pass: collect all specs recursively
	walkErr := filepath.WalkDir(sdg.loader.specsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil {
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".yaml") {
			return nil
		}
		// Skip macOS AppleDouble/resource-fork files (._*)
		if appledouble.SkipNameInReadDir(d.Name()) {
			return nil
		}

		info, err := sdg.parseSpecInfo(path)
		if err != nil {
			// Log warning but skip files that can't be parsed
			eventLogger := logging.NewEventLogger(pkgctx.NewSystemContext())
			eventLogger.LogWarning("Failed to parse spec info",
				logging.String("file", path),
				logging.Error(err))
			return nil
		}

		if info.Ontology != emptyValue {
			sdg.specs[info.Ontology] = info
		}
		return nil
	})
	if walkErr != nil {
		return errfmt.Newf("failed to walk spec directory").Wrap(walkErr)
	}

	// Second pass: build edges (extends and composes — parent/mixin before child)
	for ontology, info := range sdg.specs {
		for _, parent := range specDependencyParents(info.Extends, info.Composes) {
			if _, exists := sdg.edges[parent]; !exists {
				sdg.edges[parent] = []string{}
			}
			already := false
			for _, dep := range sdg.edges[parent] {
				if dep == ontology {
					already = true
					break
				}
			}
			if !already {
				sdg.edges[parent] = append(sdg.edges[parent], ontology)
			}
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
	data, err := fileutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var partial struct {
		Ontology string   `yaml:"ontology"`
		Extends  string   `yaml:"extends"`
		Composes []string `yaml:"composes"`
	}

	if err := yaml.Unmarshal(data, &partial); err != nil {
		return nil, err
	}

	return &SpecInfo{
		Ontology: partial.Ontology,
		Extends:  partial.Extends,
		Composes: append([]string(nil), partial.Composes...),
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
		for _, parent := range specDependencyParents(info.Extends, info.Composes) {
			if _, exists := sdg.specs[parent]; !exists {
				kind := "missing_parent"
				rel := "extends"
				if parent != info.Extends {
					kind = "missing_compose"
					rel = "composes"
				}
				errors = append(errors, &GraphValidationError{
					Type:    kind,
					Message: fmt.Sprintf("Spec '%s' %s non-existent spec '%s'", ontology, rel, parent),
					Specs:   []string{ontology, parent},
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
		inDegree[ontology] = len(specDependencyParents(info.Extends, info.Composes))
	}

	// Kahn's algorithm for topological sort
	var queue []string
	var result []*Spec

	// Start with nodes that have no dependencies (in-degree = 0) sorted alphabetically for determinism
	var initial []string
	for ontology, degree := range inDegree {
		if degree == 0 {
			initial = append(initial, ontology)
		}
	}
	sort.Strings(initial)
	queue = append(queue, initial...)

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

		// Process dependents deterministically
		var readyDependents []string
		for _, dependent := range sdg.edges[current] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				readyDependents = append(readyDependents, dependent)
			}
		}
		sort.Strings(readyDependents)
		queue = append(queue, readyDependents...)
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

// specDependencyParents is the extends parent plus unique composes mixins.
func specDependencyParents(extends string, composes []string) []string {
	var parents []string
	seen := make(map[string]bool)
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == emptyValue || p == "null" || seen[p] {
			return
		}
		seen[p] = true
		parents = append(parents, p)
	}
	add(extends)
	for _, c := range composes {
		add(c)
	}
	return parents
}
