package zqkdev

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/zqk-os/zqk/pkg/objects"
)

func findProjectRoot() (string, error) {
	// Try ZQK_ROOT env var
	if root := zqkenv.Root().Get(); root != EmptyValue {
		return root, nil
	}

	// Try to find from current working directory
	wd, err := fileutil.Getwd()
	if err != nil {
		return "", err
	}

	// Walk up to find .zqk or go.mod
	dir := wd
	for {
		if _, err := fileutil.Stat(filepath.Join(dir, paths.ProjectDataDir)); err == nil {
			return dir, nil
		}
		if _, err := fileutil.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("could not find project root")
}

// These tests are designed to reproduce and diagnose hangs in the generate-builders command.
// They use timeouts to detect hangs and provide detailed logging to identify where execution
// gets stuck. If a test times out, check the logs to see which step was executing.
//
// Note: These tests may pass in isolation but the actual command may still hang due to:
// - Environment-specific state (filesystem, existing files, storage state)
// - Background processes or workers that exist in production but not in tests
// - Race conditions that only occur under specific timing conditions
//
// If the tests pass but the command still hangs, use SIGUSR1 diagnostics to capture
// goroutine dumps and identify the blocking point.

// TestGenerateBuilders_HangReproduction reproduces the hang in generate-builders
// This test uses a timeout to detect if the command hangs, helping identify where the issue occurs.
func TestGenerateBuilders_HangReproduction(t *testing.T) {
	// Skip in short mode - this is a long-running test that reproduces a hang
	if testing.Short() {
		t.Skip("Skipping hang reproduction test in short mode")
	}

	// Use actual project specs directory to reproduce the real scenario
	projectRoot, err := findProjectRoot()
	if err != nil {
		t.Fatalf("Failed to find project root: %v", err)
	}
	specsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)

	// Verify specs directory exists
	if _, err := fileutil.Stat(specsDir); fileutil.IsNotExist(err) {
		t.Skipf("Specs directory not found at %s, skipping test", specsDir)
	}

	t.Setenv(zqkenv.SkipSpecSchemaValidation().Name(), "1")

	// Create a context with timeout to detect hangs
	// Use a longer timeout to match the real scenario - if it hangs, we'll catch it here
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Channel to signal completion
	done := make(chan error, 1)

	// Run the generate-builders logic in a goroutine
	goroutinelabels.NewGoroutine("zqkdev_test", "generate builders hang reproduction").StartSimple(func() {
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("panic: %v", r)
			}
		}()

		// Step 1: Create spec loader
		t.Log("Step 1: Creating spec loader...")
		specLoader := objects.NewSpecLoader(specsDir)

		// Step 2: Get load order (this is where the hang might occur)
		t.Log("Step 2: Getting load order...")
		specs, err := specLoader.GetLoadOrder()
		if err != nil {
			done <- fmt.Errorf("failed to get spec load order: %w", err)
			return
		}
		t.Logf("Step 2 complete: Got %d specs in load order", len(specs))

		// Step 3: Sort by ontology
		t.Log("Step 3: Sorting specs by ontology...")
		sort.Slice(specs, func(i, j int) bool {
			return specs[i].Ontology < specs[j].Ontology
		})
		t.Log("Step 3 complete: Sorting done")

		// Step 4: Create constants factory
		t.Log("Step 4: Creating constants factory...")
		constantsFactory := builders.NewConstantsFactoryWithSpecLoader("bldr_v2", specLoader)
		t.Log("Step 4 complete: Constants factory created")

		// Step 5: Process specs (this might also hang)
		//
		// Mirror the real layout inside a temp root rather than writing to projectRoot. Generating
		// into the repository rewrote 48 hand-maintained files under pkg/specbuilder/bldr_v2 on
		// every run (the constants factory writes to outputDir's sibling bldr_v2), left 3 untracked
		// files that redeclared existing constants, and broke `go build ./...` — pkg/hive/media
		// stopped compiling because the regenerated names disagreed with the committed ones.
		// Reading the real specs is what makes this a faithful hang reproduction; writing to the
		// real tree was never part of that. Overwrite is still exercised, because the shared
		// constants files are rewritten repeatedly as the loop walks every spec.
		outputDir := filepath.Join(t.TempDir(), "pkg", "specbuilder", "builders")
		if err := fileutil.MkdirAll(outputDir, paths.DirPerm755); err != nil {
			done <- fmt.Errorf("failed to create output directory: %w", err)
			return
		}

		t.Logf("Step 5: Processing %d specs with overwrite=true (outputDir: %s)...", len(specs), outputDir)
		processed := 0
		for i, spec := range specs {
			if i%10 == 0 {
				t.Logf("Processing spec %d/%d: %s", i+1, len(specs), spec.Ontology)
			}

			// Determine YAML file path
			ontology := spec.Ontology
			if ontology == EmptyValue {
				t.Logf("Skipping spec %d with empty ontology", i)
				continue
			}

			yamlPath := filepath.Join(specsDir, ontology+".yaml")
			if _, err := fileutil.Stat(yamlPath); fileutil.IsNotExist(err) {
				yamlPath = filepath.Join(specsDir, ontology+".yml")
				if _, err := fileutil.Stat(yamlPath); fileutil.IsNotExist(err) {
					t.Logf("YAML file not found for ontology %s", ontology)
					continue
				}
			}

			// Generate builder with overwrite=true (this might hang)
			t.Logf("Generating builder for %s (overwrite)...", ontology)
			if err := builders.GenerateBuilderFromYAML(yamlPath, outputDir, "", constantsFactory); err != nil {
				t.Logf("Failed to generate builder for %s: %v", ontology, err)
				// Continue processing other specs
				continue
			}

			processed++
		}

		t.Logf("Step 5 complete: Processed %d specs", processed)
		done <- nil
	})

	// Wait for completion or timeout
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Generate-builders logic failed: %v", err)
		}
		t.Log("Test completed successfully - no hang detected")
	case <-ctx.Done():
		t.Fatalf("Test timed out after 2 minutes - this indicates a hang in generate-builders. " +
			"Check the logs above to see which step was executing when the timeout occurred.")
	}
}

// TestGenerateBuilders_GetLoadOrder_Hang tests specifically GetLoadOrder() which might be hanging
func TestGenerateBuilders_GetLoadOrder_Hang(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping hang reproduction test in short mode")
	}

	projectRoot, err := findProjectRoot()
	if err != nil {
		t.Fatalf("Failed to find project root: %v", err)
	}
	specsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)

	if _, err := fileutil.Stat(specsDir); fileutil.IsNotExist(err) {
		t.Skipf("Specs directory not found at %s, skipping test", specsDir)
	}

	t.Setenv(zqkenv.SkipSpecSchemaValidation().Name(), "1")

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	done := make(chan error, 1)

	goroutinelabels.NewGoroutine("zqkdev_test", "get load order hang test").StartSimple(func() {
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("panic: %v", r)
			}
		}()

		t.Log("Creating spec loader...")
		specLoader := objects.NewSpecLoader(specsDir)

		t.Log("Calling GetLoadOrder()...")
		specs, err := specLoader.GetLoadOrder()
		if err != nil {
			done <- fmt.Errorf("GetLoadOrder failed: %w", err)
			return
		}

		t.Logf("GetLoadOrder() completed successfully with %d specs", len(specs))
		done <- nil
	})

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("GetLoadOrder() failed: %v", err)
		}
		t.Log("GetLoadOrder() completed without hanging")
	case <-ctx.Done():
		t.Fatalf("GetLoadOrder() timed out after 1 minute - this indicates a hang in GetLoadOrder(). " +
			"This is likely in BuildGraph(), TopologicalSort(), or LoadSpecWithInheritance().")
	}
}

// TestGenerateBuilders_FullCLIExecution tests the full CLI command execution including async progress wrapper
// This reproduces the exact scenario that hangs: running the actual cobra command with --overwrite
func TestGenerateBuilders_FullCLIExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping full CLI execution test in short mode")
	}

	projectRoot, err := findProjectRoot()
	if err != nil {
		t.Fatalf("Failed to find project root: %v", err)
	}

	t.Setenv(zqkenv.SkipSpecSchemaValidation().Name(), "1")

	// Create the actual command (this includes BindAsyncProgress wrapper)
	cmd := NewGenerateCommandBuildersCmd()
	cmd.SetArgs([]string{"--overwrite"})

	// Set project root context
	ctx := context.Background()

	// Set working directory to project root
	originalWd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer func() {
		_ = fileutil.Chdir(originalWd)
	}()

	if err := fileutil.Chdir(projectRoot); err != nil {
		t.Fatalf("Failed to change to project root: %v", err)
	}

	// Create a context with timeout to detect hangs
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd.SetContext(ctx)

	// Channel to signal completion
	done := make(chan error, 1)

	// Run command in goroutine
	goroutinelabels.NewGoroutine("zqkdev_test", "full cli execution hang test").StartSimple(func() {
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("panic: %v", r)
			}
		}()

		t.Log("Executing generate-builders command with --overwrite...")
		err := cmd.Execute()
		done <- err
	})

	// Wait for completion or timeout
	select {
	case err := <-done:
		if err != nil {
			// Check if it's a context timeout error (which indicates a hang)
			if ctx.Err() == context.DeadlineExceeded {
				t.Fatalf("Command timed out after 5 minutes - this indicates a hang in generate-builders CLI execution")
			}
			t.Fatalf("Command failed: %v", err)
		}
		t.Log("Command completed successfully - no hang detected")
	case <-ctx.Done():
		t.Fatalf("Test timed out after 5 minutes - this indicates a hang in generate-builders CLI execution. " +
			"The command executor goroutine may be blocked waiting for storage operations or audit events.")
	}
}
