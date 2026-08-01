package testing

import (
	"os"
	"path/filepath"

	"github.com/lanceman/zqk/pkg/errfmt"
	"gopkg.in/yaml.v3"
)

// TestScenario represents a complete test scenario
type TestScenario struct {
	Name              string     `yaml:"name"`
	Description       string     `yaml:"description,omitempty"`
	Imports           []string   `yaml:"imports,omitempty"`            // Files to import and merge
	Setup             []TestStep `yaml:"setup,omitempty"`              // Setup steps (run before tests)
	Tests             []TestStep `yaml:"tests"`                        // Test steps
	Cleanup           []TestStep `yaml:"cleanup,omitempty"`            // Cleanup steps (run after tests)
	Variables         any        `yaml:"variables,omitempty"`          // Variables for interpolation
	ResponseProcessor string     `yaml:"response_processor,omitempty"` // Processor name: "elicitation_to_success", "ignore_errors", "noop"
}

// TestStep represents a single test step (tool call)
type TestStep struct {
	Name        string           `yaml:"name"`
	Description string           `yaml:"description,omitempty"`
	Tool        string           `yaml:"tool"`                   // MCP tool name
	Args        map[string]any   `yaml:"args,omitempty"`         // Tool arguments
	Import      string           `yaml:"import,omitempty"`       // Import external data file
	ImportPath  string           `yaml:"import_path,omitempty"`  // JSONPath-like path to merge (e.g., "args.milestone_refs")
	Skip        bool             `yaml:"skip,omitempty"`         // Skip this test
	Expected    *TestExpectation `yaml:"expected,omitempty"`     // Expected result
	StoreResult string           `yaml:"store_result,omitempty"` // Store result in variables (key name)
	DependsOn   []string         `yaml:"depends_on,omitempty"`   // Dependencies on stored results
}

// TestExpectation defines what to validate in the result
type TestExpectation struct {
	Success      *bool             `yaml:"success,omitempty"`        // Expected success/failure
	HasField     map[string]any    `yaml:"has_field,omitempty"`      // Field name -> expected value
	HasFields    []string          `yaml:"has_fields,omitempty"`     // List of fields that must exist
	NotHasFields []string          `yaml:"not_has_fields,omitempty"` // List of fields that must not exist
	Matches      map[string]string `yaml:"matches,omitempty"`        // Field name -> regex pattern
	Equals       map[string]any    `yaml:"equals,omitempty"`         // Field name -> exact value
	Error        *ErrorExpectation `yaml:"error,omitempty"`          // Expected error
}

// ErrorExpectation defines expected error conditions
type ErrorExpectation struct {
	Code    *int   `yaml:"code,omitempty"`
	Message string `yaml:"message,omitempty"` // Exact message or regex pattern
	Type    string `yaml:"type,omitempty"`    // Error type (e.g., "elicitation", "validation")
}

// ScenarioLoader loads and resolves test scenarios with imports
type ScenarioLoader struct {
	baseDir string
	visited map[string]bool // Track visited imports to prevent cycles
}

// NewScenarioLoader creates a new scenario loader
func NewScenarioLoader(baseDir string) *ScenarioLoader {
	return &ScenarioLoader{
		baseDir: baseDir,
		visited: make(map[string]bool),
	}
}

// LoadScenario loads a test scenario from a file, resolving all imports
func (sl *ScenarioLoader) LoadScenario(filename string) (*TestScenario, error) {
	// Reset visited map for new load
	sl.visited = make(map[string]bool)

	scenario, err := sl.loadScenarioFile(filename)
	if err != nil {
		return nil, errfmt.Newf("failed to load scenario").Wrap(err)
	}

	// Resolve imports
	if err := sl.resolveImports(scenario, filepath.Dir(filename)); err != nil {
		return nil, errfmt.Newf("failed to resolve imports").Wrap(err)
	}

	return scenario, nil
}

// loadScenarioFile loads a single scenario file
func (sl *ScenarioLoader) loadScenarioFile(filename string) (*TestScenario, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, errfmt.Newf("failed to read file").Wrap(err)
	}

	var scenario TestScenario
	if err := yaml.Unmarshal(data, &scenario); err != nil {
		return nil, errfmt.Newf("failed to parse YAML").Wrap(err)
	}

	return &scenario, nil
}

// resolveImports resolves all imports in a scenario
func (sl *ScenarioLoader) resolveImports(scenario *TestScenario, baseDir string) error {
	// Resolve top-level imports
	for _, importPath := range scenario.Imports {
		if err := sl.resolveImport(scenario, importPath, baseDir, ""); err != nil {
			return errfmt.Errorf("failed to resolve import %s: %w", importPath, err)
		}
	}

	// Resolve step-level imports
	if err := sl.resolveStepImports(scenario.Setup, baseDir); err != nil {
		return err
	}
	if err := sl.resolveStepImports(scenario.Tests, baseDir); err != nil {
		return err
	}
	if err := sl.resolveStepImports(scenario.Cleanup, baseDir); err != nil {
		return err
	}

	return nil
}

// resolveStepImports resolves imports in a slice of steps
func (sl *ScenarioLoader) resolveStepImports(steps []TestStep, baseDir string) error {
	for i := range steps {
		if steps[i].Import != emptyValue {
			if err := sl.resolveStepImport(&steps[i], steps[i].Import, baseDir); err != nil {
				return errfmt.Errorf("failed to resolve import for step %s: %w", steps[i].Name, err)
			}
		}
	}
	return nil
}

// resolveStepImport resolves an import for a single step
func (sl *ScenarioLoader) resolveStepImport(step *TestStep, importPath string, baseDir string) error {
	// Resolve import file path
	fullPath := filepath.Join(baseDir, importPath)
	if !filepath.IsAbs(importPath) {
		absPath, err := filepath.Abs(fullPath)
		if err != nil {
			return errfmt.Newf("failed to resolve import path").Wrap(err)
		}
		fullPath = absPath
	}

	// Load imported data
	importData, err := sl.loadImportData(fullPath)
	if err != nil {
		return err
	}

	// Merge into step args based on ImportPath
	if step.ImportPath != emptyValue {
		// Merge at specific path (e.g., "args.milestone_refs")
		if err := sl.mergeAtPath(step, step.ImportPath, importData); err != nil {
			return errfmt.Errorf("failed to merge at path %s: %w", step.ImportPath, err)
		}
	} else {
		// Merge at root of args
		if step.Args == nil {
			step.Args = make(map[string]any)
		}
		if err := sl.mergeMaps(step.Args, importData); err != nil {
			return errfmt.Newf("failed to merge import data").Wrap(err)
		}
	}

	return nil
}

// resolveImport resolves a top-level import
func (sl *ScenarioLoader) resolveImport(scenario *TestScenario, importPath string, baseDir string, targetPath string) error {
	// Check for cycles
	absPath, err := filepath.Abs(filepath.Join(baseDir, importPath))
	if err != nil {
		return errfmt.Newf("failed to resolve import path").Wrap(err)
	}

	if sl.visited[absPath] {
		return errfmt.Errorf("circular import detected: %s", absPath)
	}
	sl.visited[absPath] = true
	defer delete(sl.visited, absPath)

	// Load imported scenario or data
	importData, err := sl.loadImportData(absPath)
	if err != nil {
		return err
	}

	// Try to load as scenario first (for recursive scenario imports)
	if importedScenario, err := sl.loadScenarioFile(absPath); err == nil {
		// Recursively resolve imports of the imported scenario
		if err := sl.resolveImports(importedScenario, filepath.Dir(absPath)); err != nil {
			return errfmt.Newf("failed to resolve imports in imported scenario").Wrap(err)
		}
		// Merge scenario (merge tests, setup, etc.)
		return sl.mergeScenarios(scenario, importedScenario)
	}

	// Otherwise merge as data
	if targetPath != emptyValue {
		// Merge at specific path
		return sl.mergeAtPath(scenario, targetPath, importData)
	}

	// Merge into variables
	if scenario.Variables == nil {
		scenario.Variables = make(map[string]any)
	}
	vars, ok := scenario.Variables.(map[string]any)
	if !ok {
		return errfmt.Errorf("variables must be a map for importing")
	}
	return sl.mergeMaps(vars, importData)
}

// loadImportData loads data from an import file (YAML or JSON)
func (sl *ScenarioLoader) loadImportData(filename string) (map[string]any, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, errfmt.Newf("failed to read import file").Wrap(err)
	}

	var result map[string]any
	if err := yaml.Unmarshal(data, &result); err != nil {
		return nil, errfmt.Newf("failed to parse import file").Wrap(err)
	}

	return result, nil
}

// mergeScenarios merges an imported scenario into the base scenario
func (sl *ScenarioLoader) mergeScenarios(base, imported *TestScenario) error {
	// Merge tests (append)
	base.Tests = append(base.Tests, imported.Tests...)

	// Merge setup (prepend - imported setup runs first)
	base.Setup = append(imported.Setup, base.Setup...)

	// Merge cleanup (append - imported cleanup runs first)
	base.Cleanup = append(imported.Cleanup, base.Cleanup...)

	// Merge variables
	if imported.Variables != nil {
		if base.Variables == nil {
			base.Variables = imported.Variables
		} else {
			baseVars, ok1 := base.Variables.(map[string]any)
			importVars, ok2 := imported.Variables.(map[string]any)
			if ok1 && ok2 {
				if err := sl.mergeMaps(baseVars, importVars); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// mergeAtPath merges data at a specific path (e.g., "args.milestone_refs")
func (sl *ScenarioLoader) mergeAtPath(target any, path string, data any) error {
	// Simple implementation: split path and navigate
	// For now, only support single-level paths (e.g., "args")
	// TODO: Support nested paths (e.g., "args.milestone_refs")

	// This is a simplified version - could be enhanced with proper JSONPath
	if path == "args" {
		if step, ok := target.(*TestStep); ok {
			if step.Args == nil {
				step.Args = make(map[string]any)
			}
			if dataMap, ok := data.(map[string]any); ok {
				return sl.mergeMaps(step.Args, dataMap)
			}
		}
	}

	return errfmt.Errorf("unsupported merge path: %s", path)
}

// mergeMaps merges two maps (target gets overwritten by source)
func (sl *ScenarioLoader) mergeMaps(target, source map[string]any) error {
	for k, v := range source {
		target[k] = v
	}
	return nil
}
