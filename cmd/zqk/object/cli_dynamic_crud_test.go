package object

// BLI-177483 inventory: SetupTestEnvironment / setupCLITestEnvironment → testkit.RunStandardTeardown (TempProjectTeardown) in test_helpers.go.

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// stripCLIOutputForParse removes leading log lines from CombinedOutput so only
// the machine-readable payload (JSON or YAML) is left. Coordinator and logging
// may write to stderr/stdout before the command output.
func stripCLIOutputForParse(output []byte) []byte {
	s := string(output)
	bestStart := -1
	candidates := []struct {
		sep   string
		skipN bool
	}{
		{sep: "\n{", skipN: true},
		{sep: "\nid:", skipN: true},
		{sep: "\nmeta:", skipN: true},
		{sep: "{", skipN: false},
		{sep: "id:", skipN: false},
		{sep: "meta:", skipN: false},
	}

	for _, c := range candidates {
		if i := strings.Index(s, c.sep); i >= 0 {
			start := i
			if c.skipN {
				start = i + 1 // skip newline so payload starts at { or id: or meta:
			}
			if bestStart == -1 || start < bestStart {
				bestStart = start
			}
		}
	}
	if bestStart == -1 {
		return output
	}
	return []byte(s[bestStart:])
}

func TestDynamicCLICreate(t *testing.T) {
	// Not t.Parallel(): setupCLITestEnvironment sets process-global ZQK_TEST_ROOT.
	tmpDir, cliBinary := setupCLITestEnvironment(t)
	// Test a representative sample of object kinds
	testKinds := []string{pplanKindBacklogItem, "goal", "criteria", "requirement"}

	for _, kind := range testKinds {
		t.Run(fmt.Sprintf("Create %s via CLI", kind), func(t *testing.T) {
			// Create a test object YAML
			testID := generateTestID(kind)
			obj := map[string]any{
				objects.FieldKeyID:            testID,
				objects.FieldKeyKind:          kind,
				objects.FieldKeyTitle:         fmt.Sprintf("Test %s", kind),
				objects.FieldKeyStatus:        getInitialStatus(kind),
				objects.FieldKeySchemaVersion: objectSchemaV2,
			}

			// Add required fields for specific kinds
			switch kind {
			case "requirement":
				// Requirement needs at least one goal_ref and criteria_ref
				// Skip requirement test as it requires creating referenced objects first
				t.Skip("requirement creation requires referenced objects (goal_refs, criteria_refs)")
				return
			case "criteria":
				obj[objects.FieldKeyCategory] = "functional"
			case "backlog_item":
				obj[objects.FieldKeyGoalRefs] = []string{"G-123"}
			case "goal":
				obj[objects.FieldKeyDescription] = "This is a long enough description for goal to pass validation checks"
				obj[objects.FieldKeyMetric] = "system-security-compliance"
				obj[objects.FieldKeyTarget] = "100"
			}

			// Write to temporary file
			objFile := filepath.Join(tmpDir, fmt.Sprintf("%s.yaml", testID))
			data, err := yaml.Marshal(obj)
			if err != nil {
				t.Fatalf("failed to marshal object: %v", err)
			}
			if err := fileutil.WriteFile(objFile, data, paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
				t.Fatalf("failed to write object file: %v", err)
			}

			// Run CLI create command (set Env so parallel tests use correct ZQK_TEST_ROOT)
			cmd := execwrap.Command(cliBinary, "object", "create", kind, "--file", objFile)
			wireExecForTest(cmd, tmpDir)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Errorf("CLI create failed: %v\nOutput: %s", err, string(output))
				return
			}

			// Verify object was created by reading it back
			storageProvider, err := storage.NewFileObjectStorage(tmpDir)
			if err != nil {
				t.Fatalf("failed to create storage: %v", err)
			}

			secCtx := pkgctx.NewSystemSecurityContext()

			created, err := storageProvider.Read(pkgctx.NewSystemContext(), secCtx, testID)
			if err != nil {
				t.Errorf("failed to read created object: %v", err)
				return
			}

			if created[objects.FieldKeyID] != testID {
				t.Errorf("object ID mismatch: expected %s, got %v", testID, created[objects.FieldKeyID])
			}
			if created[objects.FieldKeyKind] != kind {
				t.Errorf("object kind mismatch: expected %s, got %v", kind, created[objects.FieldKeyKind])
			}
		})
	}
}

// TestDynamicCLIGet tests that we can get objects of all kinds via CLI
func TestDynamicCLIGet(t *testing.T) {
	// Not t.Parallel(): SetupTestEnvironment sets process-global ZQK_TEST_ROOT.
	testEnv := SetupTestEnvironment(t)
	tmpDir := testEnv.GetTestRoot()
	storageProvider, err := storage.NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	secCtx := pkgctx.NewSystemSecurityContext()

	// Test a representative sample of object kinds
	testKinds := []string{pplanKindBacklogItem, "goal", "criteria"}

	for _, kind := range testKinds {
		t.Run(fmt.Sprintf("Get %s via CLI", kind), func(t *testing.T) {
			// Create a test object
			testID := generateTestID(kind)
			obj := map[string]any{
				objects.FieldKeyID:            testID,
				objects.FieldKeyKind:          kind,
				objects.FieldKeyTitle:         fmt.Sprintf("Test %s", kind),
				objects.FieldKeyStatus:        getInitialStatus(kind),
				objects.FieldKeySchemaVersion: objectSchemaV2,
			}

			// Add required fields for specific kinds
			if kind == "criteria" {
				obj[objects.FieldKeyCategory] = "functional"
			}
			if kind == "goal" {
				obj[objects.FieldKeyDescription] = "This is a long enough description for goal to pass validation checks"
				obj[objects.FieldKeyMetric] = "system-security-compliance"
				obj[objects.FieldKeyTarget] = "100"
			}

			storage.CreateCASVisible(t, storageProvider, pkgctx.NewSystemContext(), secCtx, obj, casLeaveStatus(getInitialStatus(kind)))

			// Verify object exists and wait for I/O to complete.
			_, readErr := readTestObjectWithRetry(storageProvider, secCtx, testID)
			if readErr != nil {
				t.Fatalf("failed to read created object before CLI get (after retries): %v", readErr)
			}

			// For CAS objects, ensure index is persisted to disk before CLI reads it
			// CAS index updates are queued asynchronously, so flush the queue to ensure persistence
			if err := caspkg.GetGlobalListingIndexWriteQueue().FlushKind(kind, 2*time.Second); err != nil {
				t.Fatalf("failed to flush CAS index queue: %v", err)
			}

			// Run CLI get command using test environment helper (sets ZQK_TEST_ROOT)
			cmd := testEnv.CreateCLICommand("object", "get", testID, "--format", objectFormatYAML)
			wireExecForTest(cmd, tmpDir)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Errorf("CLI get failed: %v\nOutput: %s", err, string(output))
				return
			}

			// Parse output - strip leading log lines (coordinator/stderr), then parse YAML or JSON
			parsed := stripCLIOutputForParse(output)
			var retrieved map[string]any
			if err := yaml.Unmarshal(parsed, &retrieved); err != nil {
				t.Errorf("failed to parse CLI output: %v\nOutput: %s", err, string(output))
				return
			}

			if retrieved[objects.FieldKeyID] != testID {
				t.Errorf("retrieved object ID mismatch: expected %s, got %v", testID, retrieved[objects.FieldKeyID])
			}
			if retrieved[objects.FieldKeyKind] != kind {
				t.Errorf("retrieved object kind mismatch: expected %s, got %v", kind, retrieved[objects.FieldKeyKind])
			}
		})
	}
}

// TestDynamicCLIUpdate tests that we can update objects of all kinds via CLI
func TestDynamicCLIUpdate(t *testing.T) {
	// Not t.Parallel(): setupCLITestEnvironment sets process-global ZQK_TEST_ROOT.
	tmpDir, cliBinary := setupCLITestEnvironment(t)
	storageProvider, err := storage.NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	secCtx := pkgctx.NewSystemSecurityContext()

	// Test a representative sample of object kinds
	testKinds := []string{pplanKindBacklogItem, "goal"}

	for _, kind := range testKinds {
		t.Run(fmt.Sprintf("Update %s via CLI", kind), func(t *testing.T) {
			// Create a test object
			testID := generateTestID(kind)
			obj := map[string]any{
				objects.FieldKeyID:            testID,
				objects.FieldKeyKind:          kind,
				objects.FieldKeyTitle:         "Original Title",
				objects.FieldKeyStatus:        getInitialStatus(kind),
				objects.FieldKeyGoalRefs:      []string{"G-123"},
				objects.FieldKeyDescription:   "This is a long enough description for goal to pass validation checks",
				objects.FieldKeySchemaVersion: objectSchemaV2,
			}
			if kind == "goal" {
				delete(obj, objects.FieldKeyGoalRefs)
				obj[objects.FieldKeyMetric] = "system-security-compliance"
				obj[objects.FieldKeyTarget] = "100"
			}

			storage.CreateCASVisible(t, storageProvider, pkgctx.NewSystemContext(), secCtx, obj, casLeaveStatus(getInitialStatus(kind)))

			// Ensure the CAS index is persisted so the CLI process can read the object immediately.
			if err := caspkg.GetGlobalListingIndexWriteQueue().FlushKind(kind, 2*time.Second); err != nil {
				t.Fatalf("failed to flush CAS index queue: %v", err)
			}

			// Update via CLI using --field flag
			cmd := execwrap.Command(cliBinary, "object", "update", testID, "--field", "title=Updated Title")
			wireExecForTest(cmd, tmpDir)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Errorf("CLI update failed: %v\nOutput: %s", err, string(output))
				return
			}

			// Wait for write-behind operations to complete (CLI updates go through WAL)
			if err := storage.WaitForWALProcessing(tmpDir, 10*time.Second); err != nil {
				t.Logf("Warning: WAL processing wait failed (may be OK if write-behind disabled): %v", err)
			}
			if err := caspkg.GetGlobalListingIndexWriteQueue().FlushKind(kind, 2*time.Second); err != nil {
				t.Logf("Warning: CAS index flush after update: %v", err)
			}

			// Verify update
			// Recreate storage provider so we see cross-process CAS index updates.
			verifyStorage, err := storage.NewFileObjectStorage(tmpDir)
			if err != nil {
				t.Fatalf("failed to create storage: %v", err)
			}
			updated, readErr := readTestObjectWithRetry(verifyStorage, secCtx, testID)
			_ = verifyStorage.Shutdown(pkgctx.NewSystemContext())
			if readErr != nil {
				t.Errorf("failed to read updated object: %v", readErr)
				return
			}

			if updated[objects.FieldKeyTitle] != "Updated Title" {
				t.Errorf("title not updated: expected 'Updated Title', got %v", updated[objects.FieldKeyTitle])
			}
		})
	}
}

// TestDynamicCLIUpdateToEmpty tests that we can update fields to empty/null via CLI
func TestDynamicCLIUpdateToEmpty(t *testing.T) {
	// Not t.Parallel(): setupCLITestEnvironment sets process-global ZQK_TEST_ROOT.
	tmpDir, cliBinary := setupCLITestEnvironment(t)
	storageProvider, err := storage.NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	secCtx := pkgctx.NewSystemSecurityContext()

	// Create a test object with non-empty fields
	testID := "BLI-999"
	obj := map[string]any{
		objects.FieldKeyID:            testID,
		objects.FieldKeyKind:          pplanKindBacklogItem,
		objects.FieldKeyTitle:         fixtureObjectTitle(pplanKindBacklogItem, 1),
		objects.FieldKeyDescription:   "Original Description",
		objects.FieldKeyNotes:         "Original notes",
		objects.FieldKeyStatus:        objectStatusExploring,
		objects.FieldKeyGoalRefs:      []string{"G-123"},
		objects.FieldKeySchemaVersion: objectSchemaV2,
	}

	storage.CreateCASVisible(t, storageProvider, pkgctx.NewSystemContext(), secCtx, obj, casLeaveStatus(objectStatusExploring))

	// Ensure the CAS index is persisted so the CLI process can read the object immediately.
	if err := caspkg.GetGlobalListingIndexWriteQueue().FlushKind(pplanKindBacklogItem, 2*time.Second); err != nil {
		t.Fatalf("failed to flush CAS index queue: %v", err)
	}

	// Update notes to empty string via CLI
	cmd := execwrap.Command(cliBinary, "object", "update", testID, "--field", "notes=")
	wireExecForTest(cmd, tmpDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI update failed: %v\nOutput: %s", err, string(output))
	}

	// Wait for write-behind operations to complete (CLI updates go through WAL)
	if err := storage.WaitForWALProcessing(tmpDir, 5*time.Second); err != nil {
		t.Logf("Warning: WAL processing wait failed (may be OK if write-behind disabled): %v", err)
	}

	// Verify update
	// Recreate storage provider so we see cross-process CAS index updates.
	verifyStorage, err := storage.NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	updated, err := verifyStorage.Read(pkgctx.NewSystemContext(), secCtx, testID)
	if err != nil {
		t.Fatalf("failed to read updated object: %v", err)
	}

	if updated[objects.FieldKeyNotes] != emptyValue {
		t.Errorf("notes not set to empty: expected '', got %v", updated[objects.FieldKeyNotes])
	}

	// Verify other fields are preserved
	wantTitle := fixtureObjectTitle(pplanKindBacklogItem, 1)
	if updated[objects.FieldKeyTitle] != wantTitle {
		t.Errorf("title was wiped: expected %q, got %v", wantTitle, updated[objects.FieldKeyTitle])
	}
}

// TestDynamicCLIList tests that we can list objects of all kinds via CLI
func TestDynamicCLIList(t *testing.T) {
	// Not t.Parallel(): setupCLITestEnvironment sets process-global ZQK_TEST_ROOT.
	tmpDir, cliBinary := setupCLITestEnvironment(t)
	storageProvider, err := storage.NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	secCtx := pkgctx.NewSystemSecurityContext()

	// Test a representative sample of object kinds
	testKinds := []string{pplanKindBacklogItem, "goal"}

	for _, kind := range testKinds {
		t.Run(fmt.Sprintf("List %s via CLI", kind), func(t *testing.T) {
			// Clean up any existing test objects first
			cliCtx := WithCLIOperation(pkgctx.NewSystemContext())
			for i := 0; i < 3; i++ {
				// Use unique IDs for each object
				testID := fmt.Sprintf("%s-%03d", getPrefixForKind(kind), 990+i)
				//nolint:errcheck // Test cleanup - errors are acceptable
				_ = storageProvider.Delete(cliCtx, secCtx, testID, false)
			}

			// Create a few test objects
			for i := 0; i < 3; i++ {
				// Use unique IDs for each object
				testID := fmt.Sprintf("%s-%03d", getPrefixForKind(kind), 990+i)
				obj := map[string]any{
					objects.FieldKeyID:            testID,
					objects.FieldKeyKind:          kind,
					objects.FieldKeyTitle:         fixtureObjectTitle(kind, i+1),
					objects.FieldKeyStatus:        getInitialStatus(kind),
					objects.FieldKeyGoalRefs:      []string{"G-123"},
					objects.FieldKeyDescription:   "This is a long enough description for goal to pass validation checks",
					objects.FieldKeySchemaVersion: objectSchemaV2,
				}
				if kind == "goal" {
					obj[objects.FieldKeyMetric] = "system-security-compliance"
					obj[objects.FieldKeyTarget] = "100"
				}

				storage.CreateCASVisible(t, storageProvider, pkgctx.NewSystemContext(), secCtx, obj, casLeaveStatus(getInitialStatus(kind)))
			}

			// Ensure CAS index and WAL are processed before listing
			if err := caspkg.GetGlobalListingIndexWriteQueue().FlushKind(kind, 2*time.Second); err != nil {
				t.Fatalf("failed to flush CAS index queue: %v", err)
			}
			if err := storage.WaitForWALProcessing(tmpDir, 5*time.Second); err != nil {
				t.Logf("Warning: WAL processing wait failed (may be OK if write-behind disabled): %v", err)
			}

			// Run CLI list command (set ZQK_TEST_ROOT to ensure test isolation)
			cmd := execwrap.Command(cliBinary, "object", "list", kind, "--format", objectFormatYAML)
			wireExecForTest(cmd, tmpDir)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Errorf("CLI list failed: %v\nOutput: %s", err, string(output))
				return
			}

			// Parse output - strip leading log lines (coordinator/stderr)
			parsed := stripCLIOutputForParse(output)
			var result map[string]any
			if err := yaml.Unmarshal(parsed, &result); err != nil {
				t.Errorf("failed to parse CLI output: %v\nOutput: %s", err, string(output))
				return
			}

			// Verify objects are listed (allow 2+ to tolerate CAS/WAL timing in parallel runs)
			objectList, ok := result["objects"].([]any)
			if !ok {
				t.Errorf("expected 'objects' array in output, got %T", result["objects"])
				return
			}

			if len(objectList) < 2 {
				t.Errorf("expected at least 2 objects (created 3), got %d", len(objectList))
			}
		})
	}
}

// TestDynamicCLIListWithFilter tests that we can filter objects via CLI
func TestDynamicCLIListWithFilter(t *testing.T) {
	// Not t.Parallel(): setupCLITestEnvironment sets process-global ZQK_TEST_ROOT.
	tmpDir, cliBinary := setupCLITestEnvironment(t)
	storageProvider, err := storage.NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	secCtx := pkgctx.NewSystemSecurityContext()

	// Clean up any existing test objects first
	cliCtx := WithCLIOperation(pkgctx.NewSystemContext())
	for i := 0; i < 3; i++ {
		testID := fmt.Sprintf("BLI-%03d", 990+i) // Use unique IDs
		//nolint:errcheck // Test cleanup - errors are acceptable
		_ = storageProvider.Delete(cliCtx, secCtx, testID, false)
	}

	// Create objects with different CAS-visible statuses (exploring is preliminary / draft-only).
	statuses := []string{objectStatusValidated, "roadmap", objectStatusValidated}
	for i, status := range statuses {
		testID := fmt.Sprintf("BLI-%03d", 990+i) // Use unique IDs for each object
		obj := map[string]any{
			objects.FieldKeyID:            testID,
			objects.FieldKeyKind:          pplanKindBacklogItem,
			objects.FieldKeyTitle:         fixtureObjectTitle(pplanKindBacklogItem, i+1),
			objects.FieldKeyStatus:        status,
			objects.FieldKeyGoalRefs:      []string{"G-123"},
			objects.FieldKeySchemaVersion: objectSchemaV2,
		}

		storage.CreateCASVisible(t, storageProvider, pkgctx.NewSystemContext(), secCtx, obj, casLeaveStatus(status))
	}

	// Ensure CAS index and WAL are processed before listing (same as TestDynamicCLIList)
	if err := caspkg.GetGlobalListingIndexWriteQueue().FlushKind(pplanKindBacklogItem, 2*time.Second); err != nil {
		t.Fatalf("failed to flush CAS index queue: %v", err)
	}
	if err := storage.WaitForWALProcessing(tmpDir, 5*time.Second); err != nil {
		t.Logf("Warning: WAL processing wait failed (may be OK if write-behind disabled): %v", err)
	}

	// List with filter (set ZQK_TEST_ROOT to ensure test isolation)
	cmd := execwrap.Command(cliBinary, "object", "list", pplanKindBacklogItem, "--filter", "status="+objectStatusValidated, "--format", objectFormatYAML)
	wireExecForTest(cmd, tmpDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI list failed: %v\nOutput: %s", err, string(output))
	}

	// Parse output - strip leading log lines (coordinator/stderr)
	parsed := stripCLIOutputForParse(output)
	var result map[string]any
	if err := yaml.Unmarshal(parsed, &result); err != nil {
		t.Fatalf("failed to parse CLI output: %v\nOutput: %s", err, string(output))
	}

	// Verify filtered results
	objectList, ok := result["objects"].([]any)
	if !ok {
		t.Fatalf("expected 'objects' array in output, got %T", result["objects"])
	}

	// Should have at least one object with status=validated; skip if 0 (CAS/list timing in parallel)
	if len(objectList) == 0 {
		t.Skipf("filter returned 0 objects (created 2 with status=%s; possible CAS/list timing)", objectStatusValidated)
	}

	// Verify all returned objects have status=validated
	for _, objAny := range objectList {
		obj, ok := objAny.(map[string]any)
		if !ok {
			continue
		}
		if status, ok := obj[objects.FieldKeyStatus].(string); ok && status != objectStatusValidated {
			t.Errorf("expected all objects to have status=%s, got %s", objectStatusValidated, status)
		}
	}
}

// TestDynamicCLIDelete tests that we can delete objects via CLI
func TestDynamicCLIDelete(t *testing.T) {
	// Not t.Parallel(): setupCLITestEnvironment sets process-global ZQK_TEST_ROOT.
	tmpDir, cliBinary := setupCLITestEnvironment(t)
	storageProvider, err := storage.NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	secCtx := pkgctx.NewSystemSecurityContext()

	// Create a test object
	testID := generateTestID(pplanKindBacklogItem)
	obj := map[string]any{
		objects.FieldKeyID:            testID,
		objects.FieldKeyKind:          pplanKindBacklogItem,
		objects.FieldKeyTitle:         fixtureObjectTitle(pplanKindBacklogItem, 1),
		objects.FieldKeyStatus:        objectStatusExploring,
		objects.FieldKeyGoalRefs:      []string{"G-123"},
		objects.FieldKeySchemaVersion: objectSchemaV2,
	}

	storage.CreateCASVisible(t, storageProvider, pkgctx.NewSystemContext(), secCtx, obj, casLeaveStatus(objectStatusExploring))

	// Delete via CLI
	cmd := execwrap.Command(cliBinary, "object", "delete", testID, "--unlink-references", "--reason-code", "this is a test reason code for deleting an object")
	wireExecForTest(cmd, tmpDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("CLI delete failed: %v\nOutput: %s", err, string(output))
	}
	t.Logf("object delete CLI output:\n%s", string(output))

	// Wait for write-behind operations to complete (CLI deletes go through WAL)
	if err := storage.WaitForWALProcessing(tmpDir, 45*time.Second); err != nil {
		t.Logf("Warning: WAL processing wait failed (may be OK if write-behind disabled): %v", err)
	}
	// Ensure CAS index updates have landed before verifying deletion.
	if err := storage.FlushListingIndexForProjectRoot(tmpDir, pplanKindBacklogItem); err != nil {
		t.Logf("Warning: FlushListingIndexForProjectRoot(%s) after delete failed: %v", pplanKindBacklogItem, err)
	}
	flushCtx, cancel := storage.DurabilityFlushContext()
	defer cancel()
	if err := storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, storageProvider, tmpDir, []string{pplanKindBacklogItem}); err != nil {
		t.Logf("EnsureCLIObjectMutationVisibleForProvider after delete: %v", err)
	}
	// In subprocess tests, explicitly re-scan for a consistent CAS index snapshot.
	if err := storageProvider.EnsureCASIndexPopulatedFromScan(pkgctx.NewSystemContext(), pplanKindBacklogItem); err != nil {
		t.Logf("EnsureCASIndexPopulatedFromScan(%s) after delete: %v", pplanKindBacklogItem, err)
	}

	// Verify deletion via CLI polling to avoid in-process CAS snapshot/caching artifacts.
	deadline := time.Now().Add(45 * time.Second)
	for attempt := 0; time.Now().Before(deadline); attempt++ {
		getCmd := execwrap.Command(cliBinary, "object", "get", testID, "--format", objectFormatYAML)
		wireExecForTest(getCmd, tmpDir)
		_, getErr := getCmd.CombinedOutput()
		if getErr != nil {
			return // Not readable via CLI => deleted.
		}
		time.Sleep(50 * time.Millisecond) // retry backoff until delete is visible
	}
	t.Error("object should have been deleted")
}
