package testing

import (
	"path/filepath"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// getMapKeys returns all keys from a map (for debugging)
func getMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// TestInteractiveCreationAllKinds tests creating one object of each kind using interactive creation
func TestInteractiveCreationAllKinds(t *testing.T) {
	// Not t.Parallel(): PrepareIsolatedTempProject uses t.Setenv(ZQK_TEST_ROOT).
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{Kind: "mcp.testing.interactive_all_kinds"})
	testRoot := proj.Root

	// Create MCP server
	server := mcp.NewServer()

	// Setup security context (admin for full access)
	secCtx := &pkgctx.SecurityContext{
		AccountID:   "test-account",
		Roles:       []string{"admin"},
		Permissions: []string{"*"},
	}
	server.SetSecurityContext(secCtx)

	// Get root command for CLI bridge (if available)
	// For testing, we can skip this and just register built-in tools

	// Register tools
	mcp.RegisterInteractiveTools(server)
	mcp.RegisterCommonTools(server)

	// Create executor
	executor := NewScenarioExecutor(server)

	// Create initialization context
	initCtx := pkgctx.NewCliInitializationContext(func(string) string { return testRoot }, testRoot)
	server.SetCliInitializationContext(initCtx)

	// Get project root (for loading scenarios)
	cwd, _ := fileutil.Getwd()
	projectRoot := filepath.Join(cwd, "..", "..", "..")

	// Load test scenario
	scenarioPath := filepath.Join(projectRoot, "test-scenarios", "interactive", "all-kinds.yaml")
	if _, err := fileutil.Stat(scenarioPath); fileutil.IsNotExist(err) {
		t.Skipf("Test scenario not found: %s", scenarioPath)
	}

	loader := NewScenarioLoader(filepath.Dir(scenarioPath))
	scenario, err := loader.LoadScenario(scenarioPath)
	if err != nil {
		t.Fatalf("Failed to load test scenario: %v", err)
	}

	// Run scenario
	results, err := executor.RunScenario(scenario)
	if err != nil {
		// Print debug info for first failing step
		if results != nil && len(results.StepResults) > 0 {
			firstStep := results.StepResults[0]
			t.Logf("First step debug - Name: %s, Success: %v", firstStep.StepName, firstStep.Success)
			if firstStep.Error != nil {
				t.Logf("  Error: %v (type: %T)", firstStep.Error, firstStep.Error)
			}
			if firstStep.Result != nil {
				t.Logf("  Result type: %T", firstStep.Result)
				if resultMap, ok := firstStep.Result.(map[string]any); ok {
					for k, v := range resultMap {
						t.Logf("  Result[%s] = %v (type: %T)", k, v, v)
					}
				}
			}
		}
		t.Fatalf("Scenario execution failed: %v", err)
	}

	// Check results
	if results == nil {
		t.Fatal("Results should not be nil")
	}
	t.Logf("Scenario: %s", results.ScenarioName)
	t.Logf("Steps executed: %d", len(results.StepResults))

	for _, stepResult := range results.StepResults {
		t.Logf("Step: %s, Tool: %s, Success: %v", stepResult.StepName, stepResult.Tool, stepResult.Success)
		if stepResult.Error != nil {
			t.Logf("  Error: %v", stepResult.Error)
		}
		if stepResult.Result != nil {
			if resultMap, ok := stepResult.Result.(map[string]any); ok {
				t.Logf("  Result keys: %v", getMapKeys(resultMap))
			}
		}
	}
}

// TestInteractiveCreationFromFile tests loading a scenario from a file
func TestInteractiveCreationFromFile(t *testing.T) {
	// Not t.Parallel(): PrepareIsolatedTempProject uses t.Setenv(ZQK_TEST_ROOT).
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{Kind: "mcp.testing.interactive_from_file"})
	testRoot := proj.Root

	// Get project root
	cwd, _ := fileutil.Getwd()
	projectRoot := filepath.Join(cwd, "..", "..", "..")

	// Create MCP server
	server := mcp.NewServer()

	// Setup security context
	secCtx := &pkgctx.SecurityContext{
		AccountID:   "test-account",
		Roles:       []string{"admin"},
		Permissions: []string{"*"},
	}
	server.SetSecurityContext(secCtx)

	// Register tools
	mcp.RegisterInteractiveTools(server)
	mcp.RegisterCommonTools(server)

	// Create executor
	executor := NewScenarioExecutor(server)

	// Load scenario from examples
	scenarioPath := filepath.Join(projectRoot, "test-scenarios", "examples", "interactive-creation.yaml")
	if _, err := fileutil.Stat(scenarioPath); fileutil.IsNotExist(err) {
		t.Skipf("Test scenario not found: %s", scenarioPath)
	}

	// Create initialization context
	initCtx := pkgctx.NewCliInitializationContext(func(string) string { return testRoot }, testRoot)
	server.SetCliInitializationContext(initCtx)

	loader := NewScenarioLoader(filepath.Dir(scenarioPath))
	scenario, err := loader.LoadScenario(scenarioPath)
	if err != nil {
		t.Fatalf("Failed to load test scenario: %v", err)
	}

	// Run scenario
	results, err := executor.RunScenario(scenario)
	if err != nil {
		t.Fatalf("Scenario execution failed: %v", err)
	}

	// Check results
	if results == nil {
		t.Fatal("Results should not be nil")
	}
	t.Logf("Scenario executed: %s", results.ScenarioName)
}
