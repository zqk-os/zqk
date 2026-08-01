package object

import (
	"github.com/lanceman/zqk/pkg/datacell"

	stdctx "context" // Explicitly alias standard context package
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/lanceman/zqk/internal/cli"
	clitool "github.com/lanceman/zqk/pkg/cli"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
	"github.com/spf13/cobra"
)

func writeDummySpecFile(dir, kind, content string) error {
	return os.WriteFile(filepath.Join(dir, kind+".yaml"), []byte(content), paths.FilePerm644)
}

func initBulkUpdateTestProjectLayout(root string) error {
	projectDataDir := filepath.Join(root, paths.ProjectDataDir)
	if err := os.MkdirAll(projectDataDir, paths.DirPerm755); err != nil {
		return err
	}
	processDir := datacell.ProcessPrimaryDir(root)
	if err := os.MkdirAll(processDir, paths.DirPerm755); err != nil {
		return err
	}
	backlogDir := filepath.Join(processDir, "backlog")
	requirementsDir := filepath.Join(processDir, "requirements")
	if err := os.MkdirAll(backlogDir, paths.DirPerm755); err != nil {
		return err
	}
	if err := os.MkdirAll(requirementsDir, paths.DirPerm755); err != nil {
		return err
	}
	specsDir := filepath.Join(processDir, "_internal", "object_specs")
	if err := os.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		return err
	}
	if err := writeDummySpecFile(specsDir, "base_object", `
kind: base_object
name: Base Object
version: v1
schema:
  type: object
  properties:
    id: { type: string, pattern: "^[A-Z]{3}-\\d{3}$" }
    kind: { type: string, pattern: "^[a-z_]+$" }
    title: { type: string }
    description: { type: string }
    created_at: { type: string, format: "date-time" }
    updated_at: { type: string, format: "date-time" }
`); err != nil {
		return err
	}
	if err := writeDummySpecFile(specsDir, "auditable", `
kind: auditable
name: Auditable
version: v1
extends: base_object
schema:
  type: object
  properties:
    created_by: { type: string }
    updated_by: { type: string }
`); err != nil {
		return err
	}
	if err := writeDummySpecFile(specsDir, pplanKindBacklogItem, `
kind: backlog_item
name: Backlog Item
version: v1
extends: auditable
schema:
  type: object
  properties:
    id: { type: string }
    kind: { type: string }
    status: { type: string }
    title: { type: string }
    priority: { type: string }
    non_existent_field: { type: string }
    priority_rank: { type: integer }
    completed: { type: boolean }
    tags: { type: string }
    labels: { type: string }
`); err != nil {
		return err
	}
	if err := writeDummySpecFile(specsDir, "requirement", `
kind: requirement
name: Requirement
version: v1
extends: auditable
schema:
  type: object
  properties:
    id: { type: string }
    kind: { type: string }
    status: { type: string }
    milestone_refs: { type: string }
    owner: { type: string }
`); err != nil {
		return err
	}
	return nil
}

// setupTestProject creates a temporary project structure for testing via [testkit.PrepareIsolatedTempProject].
func setupTestProject(t *testing.T) (tempDir string, cleanup func()) {
	testEnvMu.Lock()
	t.Cleanup(func() { testEnvMu.Unlock() })
	p := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind:                     "cmd.object.bulk_update",
		SkipSetupTestEnvironment: true,
		AppendStagesBeforeStorage: func(root string) []testkit.NamedTestStep {
			return []testkit.NamedTestStep{{
				Name: "bulk_update_layout_and_specs",
				Fn:   func() error { return initBulkUpdateTestProjectLayout(root) },
			}}
		},
	})
	return p.Root, func() {}
}

// setTestCLIContext sets up a test CLI context for the given command.
// If storageProvider is non-nil, that instance is used so the command and test share the same storage.
func setTestCLIContext(t *testing.T, cmd *cobra.Command, projectRoot string, storageProvider storage.ObjectStorageProvider) stdctx.Context {
	// Ensure flags are initialized for the command
	if cmd.PersistentFlags().Lookup("context") == nil {
		cmd.PersistentFlags().String("context", "test", "Context profile for testing")
	}

	// Step 1: Get the current standard library context from the Cobra command.
	cmdStandardContext := cmd.Context()
	if cmdStandardContext == nil {
		// Fresh commands often have nil Context(); WithStorageProvider(nil, p) returns nil and storage
		// is never attached — bulk update would then use GetObjectStorageForCommand cache/new storage
		// instead of the test provider used for Create.
		cmdStandardContext = stdctx.Background()
	}

	// Step 2: If a storage provider isn't already in this standard context, inject it.
	// Use the provided provider when non-nil so the command and test share the same instance;
	// otherwise create one (NewFileObjectStorageForTest for synchronous writes).
	if cli.GetStorageProvider(cmdStandardContext) == nil {
		var provider storage.ObjectStorageProvider
		if storageProvider != nil {
			provider = storageProvider
		} else {
			var err error
			provider, err = storage.NewFileObjectStorageForTest(projectRoot)
			if err != nil {
				t.Fatalf("Failed to create test storage provider: %v", err)
			}
		}
		cmdStandardContext = cli.WithStorageProvider(cmdStandardContext, provider)
	}

	// Step 3: Update the Cobra command's context with the one containing the storage provider.
	// This is crucial to ensure the underlying standard context is correctly set for ZQK's context loader.
	cmd.SetContext(cmdStandardContext)

	// Step 4: Create the ZQK-specific CliInitializationContext.
	initCtx := pkgctx.NewCliInitializationContext(func(s string) string { return projectRoot }, projectRoot)

	// Step 5: Use cli.GetContextFromCommand to get the ZQK cli.Context wrapper.
	// This function should now build the wrapper around the already updated cmd.Context().
	cliContextWrapper, err := cli.GetContextFromCommand(cmd, initCtx)
	if err != nil {
		t.Fatalf("Failed to get CLI context wrapper: %v", err)
	}

	// Step 6: Finally, set this fully configured cliContextWrapper back to the Cobra command.
	// This ensures that subsequent calls to cli.GetContext(cmd) will retrieve this wrapper.
	cli.SetContext(cmd, cliContextWrapper)

	// Step 7: Propagate standard context (with storage provider) to the bulk update subcommand
	// so GetObjectStorageForCommand finds the provider when the update RunE runs (cmd is the leaf).
	for _, c := range cmd.Commands() {
		if c.Name() == "object" {
			for _, c2 := range c.Commands() {
				if c2.Name() == "bulk" {
					for _, c3 := range c2.Commands() {
						if c3.Name() == "update" {
							c3.SetContext(cmdStandardContext)
							break
						}
					}
					break
				}
			}
			break
		}
	}

	return cmdStandardContext // Return the standard context
}

// TestBulkUpdateCmd tests the zqk object bulk update command
func TestBulkUpdateCmd(t *testing.T) {
	testCases := []struct {
		name         string
		setupObjects []map[string]any
		args         []string
		expectedErr  string
		expectOutput func(t *testing.T, output string)
		expectObject func(t *testing.T, provider storage.ObjectStorageProvider)
	}{
		{
			name: "successful basic update",
			setupObjects: []map[string]any{
				{objects.FieldKeyID: "ITEM-001", objects.FieldKeyKind: pplanKindBacklogItem, objects.FieldKeyStatus: objectStatusPlanned, objects.FieldKeyTitle: "Item 1"},
				{objects.FieldKeyID: "ITEM-002", objects.FieldKeyKind: pplanKindBacklogItem, objects.FieldKeyStatus: objectStatusPlanned, objects.FieldKeyTitle: "Item 2"},
			},
			args:        []string{pplanKindBacklogItem, "--filter", "status=" + objectStatusPlanned, "--set", "status=" + objectStatusInProgress},
			expectedErr: "",
			expectOutput: func(t *testing.T, output string) {
				if !strings.Contains(output, "Total:") || !strings.Contains(output, "Success:") {
					t.Errorf("Expected bulk result summary in output, got: %s", output)
				}
				if !strings.Contains(output, "2") {
					t.Errorf("Expected count 2 in output, got: %s", output)
				}
			},
			expectObject: func(t *testing.T, provider storage.ObjectStorageProvider) {
				secCtx := pkgctx.NewSystemSecurityContext()
				obj1, err := provider.Read(stdctx.Background(), secCtx, "ITEM-001")
				if err != nil {
					t.Fatalf("Failed to get ITEM-001: %v", err)
				}
				if obj1[objects.FieldKeyStatus] != objectStatusInProgress {
					t.Errorf("ITEM-001 status not updated (got %v, want %s)", obj1[objects.FieldKeyStatus], objectStatusInProgress)
				}
				obj2, err := provider.Read(stdctx.Background(), secCtx, "ITEM-002")
				if err != nil {
					t.Fatalf("Failed to get ITEM-002: %v", err)
				}
				if obj2[objects.FieldKeyStatus] != objectStatusInProgress {
					t.Errorf("ITEM-002 status not updated (got %v, want %s)", obj2[objects.FieldKeyStatus], objectStatusInProgress)
				}
			},
		},
		{
			name: "dry run update",
			setupObjects: []map[string]any{
				{objects.FieldKeyID: "ITEM-003", objects.FieldKeyKind: pplanKindBacklogItem, objects.FieldKeyStatus: objectStatusPlanned, objects.FieldKeyTitle: "Item 3"},
			},
			args:        []string{pplanKindBacklogItem, "--filter", "status=" + objectStatusPlanned, "--set", "status=" + objectStatusInProgress, "--dry-run"},
			expectedErr: "",
			expectOutput: func(t *testing.T, output string) {
				if !strings.Contains(output, "Total:") || !strings.Contains(output, "Success:") {
					t.Errorf("Expected bulk result summary in output, got: %s", output)
				}
				if !strings.Contains(output, "1") {
					t.Errorf("Expected count 1 in output, got: %s", output)
				}
			},
			expectObject: func(t *testing.T, provider storage.ObjectStorageProvider) {
				secCtx := pkgctx.NewSystemSecurityContext()
				obj3, err := provider.Read(stdctx.Background(), secCtx, "ITEM-003")
				if err != nil {
					t.Fatalf("Failed to get ITEM-003: %v", err)
				}
				if obj3[objects.FieldKeyStatus] != objectStatusPlanned {
					t.Errorf("ITEM-003 status changed in dry run (got %v, want %s)", obj3[objects.FieldKeyStatus], objectStatusPlanned)
				}
			},
		},
		{
			name: "no objects found for filter",
			setupObjects: []map[string]any{
				// backlog_item lifecycle uses "complete" (not "completed"); normalize may rewrite invalid values.
				{objects.FieldKeyID: "ITEM-004", objects.FieldKeyKind: pplanKindBacklogItem, objects.FieldKeyStatus: objectStatusComplete, objects.FieldKeyTitle: "Item 4"},
			},
			args:        []string{pplanKindBacklogItem, "--filter", "status=" + objectStatusPlanned, "--set", "status=" + objectStatusInProgress},
			expectedErr: "", // No error, just a message
			expectOutput: func(t *testing.T, output string) {
				if !strings.Contains(output, "No objects found matching the filter to update.") {
					t.Errorf("Expected 'no objects found' message, got: %s", output)
				}
			},
			expectObject: func(t *testing.T, provider storage.ObjectStorageProvider) {
				secCtx := pkgctx.NewSystemSecurityContext()
				obj4, err := provider.Read(stdctx.Background(), secCtx, "ITEM-004")
				if err != nil {
					t.Fatalf("Failed to get ITEM-004: %v", err)
				}
				if obj4[objects.FieldKeyStatus] != objectStatusComplete {
					t.Errorf("ITEM-004 status should remain '%s', got: %v", objectStatusComplete, obj4[objects.FieldKeyStatus])
				}
			},
		},
		{
			name: "invalid set format",
			setupObjects: []map[string]any{
				{objects.FieldKeyID: "ITEM-005", objects.FieldKeyKind: pplanKindBacklogItem, objects.FieldKeyStatus: objectStatusPlanned},
			},
			args:        []string{pplanKindBacklogItem, "--filter", "status=" + objectStatusPlanned, "--set", "invalid_set_format"},
			expectedErr: "invalid set format: invalid_set_format. Expected key=value",
		},
		{
			name: "missing set flag",
			setupObjects: []map[string]any{
				{objects.FieldKeyID: "ITEM-006", objects.FieldKeyKind: pplanKindBacklogItem, objects.FieldKeyStatus: objectStatusPlanned},
			},
			args:        []string{pplanKindBacklogItem, "--filter", "status=" + objectStatusPlanned},
			expectedErr: "at least one --set flag must be provided",
		},
		{
			name: "kind specified in arg conflicts with filter",
			setupObjects: []map[string]any{
				{objects.FieldKeyID: "ITEM-007", objects.FieldKeyKind: pplanKindBacklogItem, objects.FieldKeyStatus: objectStatusPlanned},
			},
			args:        []string{"requirement", "--filter", "kind=" + pplanKindBacklogItem, "--set", "status=" + objectStatusInProgress},
			expectedErr: "kind specified in argument (requirement) conflicts with kind in filter (" + pplanKindBacklogItem + ")",
		},
		{
			name: "kind inferred from id in filter",
			setupObjects: []map[string]any{
				{objects.FieldKeyID: "REQ-001", objects.FieldKeyKind: "requirement", objects.FieldKeyStatus: objectStatusDraft},
			},
			args:        []string{"requirement", "--filter", "id=REQ-001", "--set", "status=" + objectStatusFinal},
			expectedErr: "",
			expectObject: func(t *testing.T, provider storage.ObjectStorageProvider) {
				secCtx := pkgctx.NewSystemSecurityContext()
				obj, err := provider.Read(stdctx.Background(), secCtx, "REQ-001")
				if err != nil {
					t.Fatalf("Failed to get REQ-001: %v", err)
				}
				if obj[objects.FieldKeyStatus] != objectStatusFinal {
					t.Errorf("REQ-001 status not updated (got %v, want %s)", obj[objects.FieldKeyStatus], objectStatusFinal)
				}
			},
		},
		{
			name: "multiple set flags",
			setupObjects: []map[string]any{
				{objects.FieldKeyID: "ITEM-008", objects.FieldKeyKind: pplanKindBacklogItem, objects.FieldKeyStatus: objectStatusPlanned, objects.FieldKeyPriority: "medium"},
			},
			args:        []string{pplanKindBacklogItem, "--filter", "status=" + objectStatusPlanned, "--set", "status=" + objectStatusInProgress, "--set", "priority=high"},
			expectedErr: "",
			expectObject: func(t *testing.T, provider storage.ObjectStorageProvider) {
				secCtx := pkgctx.NewSystemSecurityContext()
				obj, err := provider.Read(stdctx.Background(), secCtx, "ITEM-008")
				if err != nil {
					t.Fatalf("Failed to get ITEM-008: %v", err)
				}
				if obj[objects.FieldKeyStatus] != objectStatusInProgress || obj[objects.FieldKeyPriority] != "high" {
					t.Errorf("ITEM-008 not updated (status=%v priority=%v, want %s high)", obj[objects.FieldKeyStatus], obj[objects.FieldKeyPriority], objectStatusInProgress)
				}
			},
		},
		{
			name: "update non-existent field",
			setupObjects: []map[string]any{
				{objects.FieldKeyID: "ITEM-009", objects.FieldKeyKind: pplanKindBacklogItem, objects.FieldKeyStatus: objectStatusPlanned},
			},
			args:        []string{pplanKindBacklogItem, "--filter", "status=" + objectStatusPlanned, "--set", "non_existent_field=new_value"},
			expectedErr: "", // No error for setting non-existent field, it just adds it
			expectObject: func(t *testing.T, provider storage.ObjectStorageProvider) {
				secCtx := pkgctx.NewSystemSecurityContext()
				obj, err := provider.Read(stdctx.Background(), secCtx, "ITEM-009")
				if err != nil {
					t.Fatalf("Failed to get ITEM-009: %v", err)
				}
				if obj["non_existent_field"] != "new_value" {
					t.Errorf("ITEM-009 non_existent_field not set (got %v, want new_value)", obj["non_existent_field"])
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup test environment
			tempDir, cleanup := setupTestProject(t)
			defer cleanup()

			projectRoot := tempDir

			// Manually create objects (same provider type as setTestCLIContext: synchronous writes)
			secCtx := pkgctx.NewSystemSecurityContext()
			provider, err := storage.NewFileObjectStorageForTest(projectRoot)
			if err != nil {
				t.Fatalf("Failed to create storage provider: %v", err)
				return // Return after fatalf
			}
			for _, objData := range tc.setupObjects {
				err := provider.Create(stdctx.Background(), secCtx, objData)
				if err != nil {
					t.Fatalf("Failed to create setup object %v: %v", objData, err)
					return // Return after fatalf
				}
			}

			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Create command
			rootCmd := clitool.NewCommandBuilder("zqk").Build()
			objectCmd := NewObjectCmd()
			rootCmd.AddCommand(objectCmd)

			// Set up context with same provider so command updates are visible when we Read
			testCtx := setTestCLIContext(t, rootCmd, projectRoot, provider)

			// Execute command
			fullArgs := []string{"object", "bulk", "update"}
			fullArgs = append(fullArgs, tc.args...)
			rootCmd.SetArgs(fullArgs)

			err = rootCmd.ExecuteContext(testCtx)

			// Restore stdout and read output
			w.Close()
			os.Stdout = oldStdout
			out, _ := io.ReadAll(r)
			output := string(out)

			if tc.expectedErr != emptyValue {
				if err == nil || !strings.Contains(err.Error(), tc.expectedErr) {
					t.Errorf("Expected error containing '%s', got '%v'", tc.expectedErr, err)
				}
			} else {
				if err != nil {
					t.Errorf("Did not expect error, but got: %v", err)
				}
				if tc.expectOutput != nil {
					tc.expectOutput(t, output)
				}
				if tc.expectObject != nil {
					tc.expectObject(t, provider)
				}
			}

			// Clean up any remaining objects
			for _, objData := range tc.setupObjects {
				_ = provider.Delete(stdctx.Background(), secCtx, objData[objects.FieldKeyID].(string), false) // Added missing 'false'
			}
		})
	}
}

// TestBulkUpdateMissingKindOrFilter tests validation for missing kind or filter.
func TestBulkUpdateMissingKindOrFilter(t *testing.T) {
	tempDir, cleanup := setupTestProject(t) // Use local setupTestProject
	defer cleanup()

	rootCmd := clitool.NewCommandBuilder("zqk").Build()
	objectCmd := NewObjectCmd()
	rootCmd.AddCommand(objectCmd)
	testCtx := setTestCLIContext(t, rootCmd, tempDir, nil)

	testCases := []struct {
		name        string
		args        []string
		expectedErr string
	}{
		{
			name:        "no kind and no filter",
			args:        []string{"object", "bulk", "update", "--set", "status=" + objectStatusInProgress},
			expectedErr: "kind must be specified as an argument or within a filter",
		},
		{
			name:        "no kind, filter without id",
			args:        []string{"object", "bulk", "update", "--filter", "status=" + objectStatusPlanned, "--set", "status=" + objectStatusInProgress},
			expectedErr: "could not infer kind from filter; please specify kind as an argument",
		},
		{
			name:        "kind specified as empty string",
			args:        []string{"object", "bulk", "update", "", "--filter", "id=ITEM-001", "--set", "status=" + objectStatusInProgress},
			expectedErr: "kind must be specified as an argument or within a filter",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rootCmd.SetArgs(tc.args)
			err := rootCmd.ExecuteContext(testCtx)
			if err == nil || !strings.Contains(err.Error(), tc.expectedErr) {
				t.Errorf("Expected error containing '%s', got '%v'", tc.expectedErr, err)
			}
		})
	}
}

// Helper for testing if two maps are equal, ignoring keys that might not exist in both but are not relevant.
func mapsEqual(m1, m2 map[string]any) bool {
	if len(m1) != len(m2) {
		return false
	}
	for k, v := range m1 {
		if v2, ok := m2[k]; !ok || !cmp.Equal(v, v2) {
			return false
		}
	}
	return true
}
