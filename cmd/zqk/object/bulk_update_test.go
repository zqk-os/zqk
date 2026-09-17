package object

import (
	"os"

	"github.com/lanceman/zqk/pkg/datacell"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	stdctx "context" // Explicitly alias standard context package
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/lanceman/zqk/internal/cli"
	clitool "github.com/lanceman/zqk/pkg/cli"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
)

func initBulkUpdateTestProjectLayout(root string) error {
	if err := fileutil.MkdirAll(filepath.Join(root, paths.ProjectDataDir), paths.DirPerm755); err != nil {
		return err
	}
	processDir := datacell.ProcessPrimaryDir(root)
	for _, name := range []string{"backlog", "requirements", "criteria"} {
		if err := fileutil.MkdirAll(filepath.Join(processDir, name), paths.DirPerm755); err != nil {
			return err
		}
	}
	return nil
}

// setupTestProject creates a temporary project structure for testing via [testkit.PrepareIsolatedTempProject].
// SeedSchemaPlane copies the real specs+lifecycles; the previous dummy four-spec overlay omitted
// criteria.yaml and still left FileObjectStorage bound to the real backlog_item lifecycle, so
// create (break-glass / skip-promote) could land a planned BLI that no later save could touch.
func setupTestProject(t *testing.T) (tempDir string, cleanup func()) {
	testEnvMu.Lock()
	t.Cleanup(func() { testEnvMu.Unlock() })
	p := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind:                     "cmd.object.bulk_update",
		SkipSetupTestEnvironment: true,
		SeedSchemaPlane:          true,
		AppendStagesBeforeStorage: func(root string) []testkit.NamedTestStep {
			return []testkit.NamedTestStep{{
				Name: "bulk_update_kind_dirs",
				Fn:   func() error { return initBulkUpdateTestProjectLayout(root) },
			}}
		},
	})
	return p.Root, func() {}
}

// planned is shovel-ready. CreateCASVisible skips promote when the create status is already
// non-preliminary, so a fixture written at planned lands CAS-visible without those
// preconditions ever being evaluated. Saving then revalidates the status the object already
// holds, and the CLI correctly refuses even a title-only update.
const (
	bulkFixturePlanRef      = "PRI-BULK-FIXTURE-001"
	bulkFixtureMilestoneRef = "MIL-BULK-FIXTURE-001"
	bulkFixturePriorityTier = "P1"
	bulkFixtureCriteriaID   = "CRIT-BULK-001"
)

func plannedBLI(id, title string, extra ...map[string]any) map[string]any {
	obj := map[string]any{
		objects.FieldKeyID:              id,
		objects.FieldKeyKind:            pplanKindBacklogItem,
		objects.FieldKeyStatus:          objectStatusPlanned,
		objects.FieldKeyTitle:           title,
		objects.FieldKeyPriorityPlanRef: bulkFixturePlanRef,
		objects.FieldKeyMilestoneRefs:   []string{bulkFixtureMilestoneRef},
		objects.FieldKeyPriorityTier:    bulkFixturePriorityTier,
		objects.FieldKeyCriteriaRefs:    []string{bulkFixtureCriteriaID},
	}
	for _, e := range extra {
		for k, v := range e {
			obj[k] = v
		}
	}
	return obj
}

func completeBLI(id, title string) map[string]any {
	return plannedBLI(id, title, map[string]any{
		objects.FieldKeyStatus:          objectStatusComplete,
		objects.FieldKeyEstimatedEffort: "1h",
	})
}

func seedBulkUpdateSupportObjects(t *testing.T, provider storage.ObjectStorageProvider) {
	t.Helper()
	secCtx := pkgctx.NewSystemSecurityContext()
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, map[string]any{
		objects.FieldKeyID:            bulkFixtureCriteriaID,
		objects.FieldKeyKind:          objects.KindCriteria,
		objects.FieldKeyTitle:         "Bulk fixture criterion",
		objects.FieldKeyStatus:        objects.ObjectStatusValidated,
		objects.FieldKeyCategory:      "functional",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}, objects.ObjectStatusValidated)
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
			testkit.RegisterStorageTestCleanup(t, projectRoot, provider)
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
				plannedBLI("BLI-001", "Item 1"),
				plannedBLI("BLI-002", "Item 2"),
			},
			args:        []string{pplanKindBacklogItem, "--filter", "status=" + objectStatusPlanned, "--set", "title=Updated 1"},
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
				obj1, err := provider.Read(stdctx.Background(), secCtx, "BLI-001")
				if err != nil {
					t.Fatalf("Failed to get BLI-001: %v", err)
				}
				if obj1[objects.FieldKeyTitle] != "Updated 1" {
					t.Errorf("BLI-001 title not updated (got %v, want Updated 1)", obj1[objects.FieldKeyTitle])
				}
				obj2, err := provider.Read(stdctx.Background(), secCtx, "BLI-002")
				if err != nil {
					t.Fatalf("Failed to get BLI-002: %v", err)
				}
				if obj2[objects.FieldKeyTitle] != "Updated 1" {
					t.Errorf("BLI-002 title not updated (got %v, want Updated 1)", obj2[objects.FieldKeyTitle])
				}
			},
		},
		{
			name: "refuses --set status without override",
			setupObjects: []map[string]any{
				plannedBLI("BLI-010", "Item 10"),
			},
			args:        []string{pplanKindBacklogItem, "--filter", "status=" + objectStatusPlanned, "--set", "status=" + objectStatusComplete},
			expectedErr: "manual status updates are restricted",
		},
		{
			name: "dry run update",
			setupObjects: []map[string]any{
				plannedBLI("BLI-003", "Item 3"),
			},
			args:        []string{pplanKindBacklogItem, "--filter", "status=" + objectStatusPlanned, "--set", "title=Would Update", "--dry-run"},
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
				obj3, err := provider.Read(stdctx.Background(), secCtx, "BLI-003")
				if err != nil {
					t.Fatalf("Failed to get BLI-003: %v", err)
				}
				if obj3[objects.FieldKeyStatus] != objectStatusPlanned {
					t.Errorf("BLI-003 status changed in dry run (got %v, want %s)", obj3[objects.FieldKeyStatus], objectStatusPlanned)
				}
			},
		},
		{
			name: "no objects found for filter",
			setupObjects: []map[string]any{
				completeBLI("BLI-004", "Item 4"),
			},
			args:        []string{pplanKindBacklogItem, "--filter", "status=" + objectStatusPlanned, "--set", "title=No Match"},
			expectedErr: "", // No error, just a message
			expectOutput: func(t *testing.T, output string) {
				if !strings.Contains(output, "No objects found matching the filter to update.") {
					t.Errorf("Expected 'no objects found' message, got: %s", output)
				}
			},
			expectObject: func(t *testing.T, provider storage.ObjectStorageProvider) {
				secCtx := pkgctx.NewSystemSecurityContext()
				obj4, err := provider.Read(stdctx.Background(), secCtx, "BLI-004")
				if err != nil {
					t.Fatalf("Failed to get BLI-004: %v", err)
				}
				if obj4[objects.FieldKeyStatus] != objectStatusComplete {
					t.Errorf("BLI-004 status should remain '%s', got: %v", objectStatusComplete, obj4[objects.FieldKeyStatus])
				}
			},
		},
		{
			name: "invalid set format",
			setupObjects: []map[string]any{
				plannedBLI("BLI-005", "Item 5"),
			},
			args:        []string{pplanKindBacklogItem, "--filter", "status=" + objectStatusPlanned, "--set", "invalid_set_format"},
			expectedErr: "invalid set format: invalid_set_format. Expected key=value",
		},
		{
			name: "missing set flag",
			setupObjects: []map[string]any{
				plannedBLI("BLI-006", "Item 6"),
			},
			args:        []string{pplanKindBacklogItem, "--filter", "status=" + objectStatusPlanned},
			expectedErr: "at least one --set flag must be provided",
		},
		{
			name: "kind specified in arg conflicts with filter",
			setupObjects: []map[string]any{
				plannedBLI("BLI-007", "Item 7"),
			},
			args:        []string{"requirement", "--filter", "kind=" + pplanKindBacklogItem, "--set", "status=" + objectStatusInProgress},
			expectedErr: "kind specified in argument (requirement) conflicts with kind in filter (" + pplanKindBacklogItem + ")",
		},
		{
			name: "kind inferred from id in filter",
			setupObjects: []map[string]any{
				// proposed is preliminary on requirement, so leave==proposed keeps the
				// object on the draft plane where List/bulk filters cannot see it.
				{objects.FieldKeyID: "REQ-001", objects.FieldKeyKind: "requirement", objects.FieldKeyStatus: objects.ObjectStatusActive, objects.FieldKeyTitle: "Requirement 1", objects.FieldKeyDescription: "Bulk fixture requirement"},
			},
			args:        []string{"requirement", "--filter", "id=REQ-001", "--set", "title=Inferred Kind Title"},
			expectedErr: "",
			expectObject: func(t *testing.T, provider storage.ObjectStorageProvider) {
				secCtx := pkgctx.NewSystemSecurityContext()
				obj, err := provider.Read(stdctx.Background(), secCtx, "REQ-001")
				if err != nil {
					t.Fatalf("Failed to get REQ-001: %v", err)
				}
				if obj[objects.FieldKeyTitle] != "Inferred Kind Title" {
					t.Errorf("REQ-001 title not updated (got %v, want Inferred Kind Title)", obj[objects.FieldKeyTitle])
				}
			},
		},
		{
			name: "multiple set flags",
			setupObjects: []map[string]any{
				plannedBLI("BLI-008", "Item 8", map[string]any{objects.FieldKeyPriority: "medium"}),
			},
			args:        []string{pplanKindBacklogItem, "--filter", "status=" + objectStatusPlanned, "--set", "title=High Title", "--set", "priority=high"},
			expectedErr: "",
			expectObject: func(t *testing.T, provider storage.ObjectStorageProvider) {
				secCtx := pkgctx.NewSystemSecurityContext()
				obj, err := provider.Read(stdctx.Background(), secCtx, "BLI-008")
				if err != nil {
					t.Fatalf("Failed to get BLI-008: %v", err)
				}
				if obj[objects.FieldKeyTitle] != "High Title" || obj[objects.FieldKeyPriority] != "high" {
					t.Errorf("BLI-008 not updated (title=%v priority=%v, want High Title high)", obj[objects.FieldKeyTitle], obj[objects.FieldKeyPriority])
				}
			},
		},
		{
			name: "update non-existent field",
			setupObjects: []map[string]any{
				plannedBLI("BLI-009", "Item 9"),
			},
			args:        []string{pplanKindBacklogItem, "--filter", "status=" + objectStatusPlanned, "--set", "non_existent_field=new_value"},
			expectedErr: "bulk update failed with 1 errors", // Strict schema validation rejects unknown fields
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
			testkit.RegisterStorageTestCleanup(t, projectRoot, provider)
			seedBulkUpdateSupportObjects(t, provider)
			// Create parks non-preliminary statuses on the draft plane (origin exploring);
			// promote to the intended status so List/bulk filters see the objects.
			// TRACK: BLI-1785443942668406000-1ec5c811 — draft-plane create / promote membrane.
			for _, objData := range tc.setupObjects {
				leave := objects.GetString(objData, objects.FieldKeyStatus)
				if leave == emptyValue {
					leave = objectStatusPlanned
				}
				storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, objData, leave)
			}

			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			outCh := make(chan string)
			go func() {
				out, _ := io.ReadAll(r)
				outCh <- string(out)
			}()

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
			_ = w.Close()
			os.Stdout = oldStdout
			output := <-outCh

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
			args:        []string{"object", "bulk", "update", "", "--filter", "id=BLI-001", "--set", "status=" + objectStatusInProgress},
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
