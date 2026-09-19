package system

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
)

// TestCheckCacheOnly verifies that 'zqk system check --cache-only' displays cached results
// and does not trigger a full validation scan in the CLI process.
func TestCheckCacheOnly(t *testing.T) {
	t.Parallel()

	// 1. Setup temporary project root
	tmpDir := t.TempDir()
	projectRoot := tmpDir

	// Create minimal project structure required by FileObjectStorage/AsyncValidator
	processDir := datacell.ProcessPrimaryDir(projectRoot)
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to mkdir process dir %s: %v", processDir, err)
	}
	specsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create specs directory: %v", err)
	}
	// Create a minimal spec for backlog_item so validation can process it
	specContent := `
kind: backlog_item
id_prefix: BLI
fields:
  id: {type: string, required: true}
  status: {type: string, required: true}
`
	if err := fileutil.WriteFile(filepath.Join(specsDir, "backlog_item.yaml"), []byte(specContent), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to write backlog_item spec: %v", err)
	}
	lifecyclesDir := filepath.Join(projectRoot, paths.ProcessInternalLifecyclesDir)
	if err := fileutil.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create lifecycles directory: %v", err)
	}

	// 2. Prepare a cached validation state
	// Create a dummy file that will be "validated" and cached
	testObjectID := "BLI-TEST-001"
	testContent := fmt.Sprintf("id: %s\nkind: backlog_item\nstatus: planned\n", testObjectID)
	testFilePath := testkit.WriteTestObjectStandalone(t, projectRoot, testContent)

	// Create and start AsyncValidator to populate cache
	validator := validation.NewAsyncValidator(pkgctx.NewSystemContext(), projectRoot, 1, time.Hour)
	// Set a validation function that always returns a Tier 1 issue
	validator.SetValidationFunc(func(ctx context.Context, objectID, objectKind, filePath string, content []byte) (*validation.ValidationState, error) {
		return &validation.ValidationState{
			ObjectID:      objectID,
			ObjectKind:    objectKind,
			FilePath:      filePath,
			LastValidated: time.Now(),
			Issues: []validation.ValidationIssue{
				{
					Tier:        1,
					Category:    "test",
					Message:     "Mock Tier 1 issue",
					AutoFixable: false,
					DetectedAt:  time.Now(),
				},
			},
		}, nil
	})
	if err := validator.Start(); err != nil {
		t.Fatalf("Failed to start validator: %v", err)
	}
	t.Cleanup(func() {
		if err := validator.Stop(); err != nil {
			t.Logf("Failed to stop validator: %v", err)
		}
	})

	// Enqueue the dummy file for validation
	validator.Enqueue(testObjectID, "backlog_item", testFilePath, 1)

	// Wait for validation to complete and cache to be saved
	timeout := time.After(5 * time.Second)
	tick := time.Tick(100 * time.Millisecond)
	var cachePopulated bool
	for {
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for validator to process and save cache")
		case <-tick:
			_, _, _, queueSize := validator.GetValidationStats()
			if queueSize == 0 {
				state, ok := validator.GetCachedState(testObjectID)
				if ok && state != nil && len(state.Issues) > 0 {
					t.Log("Cache populated with issue.")
					cachePopulated = true
					goto cacheLoopEnd // Break from outer loop
				}
			}
		}
	}
cacheLoopEnd:
	if !cachePopulated {
		t.Fatal("Cache was not populated with the expected issue.")
	}
	t.Log("Validator finished, cache should be populated.")
	_ = validator.Stop() // Stop validator to ensure cache is saved and not running during check

	// 3. Run 'zqk system check --cache-only'
	checkCmd := NewCheckCmd()
	checkCmd.SetArgs([]string{"--cache-only"})

	// Redirect stdout to capture output without modifying the global os.Stdout
	var buf bytes.Buffer
	checkCmd.SetOut(&buf)
	checkCmd.SetErr(&buf)

	// Set project root context for the command
	cli.SetContext(checkCmd, cli.ContextForProjectRoot(projectRoot))
	checkCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))

	// Execute the command
	err := checkCmd.Execute()
	capturedOutput := buf.String()

	var scErr *SystemCheckError
	if err != nil && !errors.As(err, &scErr) {
		t.Fatalf("Check command failed: %v\nOutput:\n%s", err, capturedOutput)
	}

	// 4. Assertions
	// Check for expected cached issue in output
	if !strings.Contains(capturedOutput, "Mock Tier 1 issue") {
		t.Errorf("Output did not contain expected cached issue:\n%s", capturedOutput)
	}
	if !strings.Contains(capturedOutput, "Layer 1: Objects Needing Fixes") {
		t.Errorf("Output did not summarize Tier 1 issues correctly:\n%s", capturedOutput)
	}

	// Assert that full validation scan messages are NOT present
	// (e.g., messages about discovering objects or progress updates)
	if strings.Contains(capturedOutput, "Validating objects") ||
		strings.Contains(capturedOutput, "Discovering objects") ||
		strings.Contains(capturedOutput, "System check progress") {
		t.Errorf("Output contained messages indicating a full validation scan:\n%s", capturedOutput)
	}
	if strings.Contains(capturedOutput, "Running validation scan") {
		t.Errorf("Output contained messages indicating a full validation scan:\n%s", capturedOutput)
	}

	t.Logf("Captured Output:\n%s", capturedOutput)
}
