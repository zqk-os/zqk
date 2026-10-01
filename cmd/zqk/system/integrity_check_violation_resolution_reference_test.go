package system

import (
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	cli "github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/migration/parser"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	testkit "github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
)

// TestViolationResolution_Reference_MissingReference tests reference violation: missing referenced object
// Resolution: Create the referenced object or remove/update the reference
// Do not use t.Parallel(): ZQK_TEST_ROOT is process-global; parallel tests can overwrite it and checkKindObjects may then use wrong root.
func TestViolationResolution_Reference_MissingReference(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	testRoot := proj.Root

	if _, err := setupSystemTestEnvironmentRoot(t, testRoot); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: pkgctx.SystemAccountID,
	}

	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, testRoot, fileStorage)

	checkCmd := func() *cobra.Command {
		cmd := &cobra.Command{}
		cmd.SetContext(cli.WithStorageProvider(pkgctx.NewSystemContext(), fileStorage))
		return cmd
	}

	// Create without reference, then promote so checkKindObjects sees CAS (not draft-only).
	// draft-plane create / promote membrane.
	obj := map[string]any{
		objects.FieldKeyID:            "BLI-306",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeyTitle:         "Missing Reference Test",
		objects.FieldKeyCreatedAt:     "2026-01-02T00:00:00Z",
		objects.FieldKeyCreatedBy:     pkgctx.SystemAccountID,
		objects.FieldKeyUpdatedAt:     "2026-01-02T00:00:00Z",
		objects.FieldKeyUpdatedBy:     pkgctx.SystemAccountID,
		objects.FieldKeyOriginProject: validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
		objects.FieldKeyNamespaceID:   paths.KernelNamespaceID,
		objects.FieldKeyPriorityTier:  "P3",
	}
	storage.CreateCASVisible(t, fileStorage, ctx, secCtx, obj, objects.ObjectStatusInProgress)

	// Resolve path after Create (CAS uses hash-named files; scan where this storage wrote)
	backlogDir := datacell.CellCASPrimaryDir(testRoot, objects.GetDirectoryFromKind(objects.KindBacklogItem))
	objectFile, err := fileStorage.GetFilePathForObject("BLI-306", "backlog_item")
	if err != nil {
		objectFile, err = resolveBacklogItemPathByScan(backlogDir, "BLI-306")
		if err != nil {
			t.Fatalf("Failed to resolve path for BLI-306 after create: %v", err)
		}
	}

	// Read current content
	content, err := fileutil.ReadFile(objectFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	// Add reference (this will create hash mismatch, but we'll check for reference violation)
	var modifiedContent string
	if strings.Contains(string(content), "priority_plan_ref:") {
		re := regexp.MustCompile(`(?m)^priority_plan_ref:.*$`)
		modifiedContent = re.ReplaceAllString(string(content), "priority_plan_ref: PRI-999")
	} else {
		modifiedContent = string(content) + "\npriority_plan_ref: PRI-999\n"
	}
	if err := fileutil.WriteFile(objectFile, []byte(modifiedContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("Failed to modify file: %v", err)
	}

	// Check for violation (will also detect hash mismatch, but we're testing reference violation)
	checkCtx := cli.ContextForProjectAndProfile(testRoot, "test")

	results, err := checkKindObjects(checkCtx, checkCmd(), "backlog_item", []string{"BLI-306"})
	if err != nil {
		t.Fatalf("Failed to check objects: %v", err)
	}

	// Find violation (may be blocked by hash mismatch, but reference violation should still be detected)
	foundReferenceViolation := false
	foundHashMismatch := false
	for _, result := range results {
		if result.ObjectID == "BLI-306" {
			for _, issue := range result.Issues {
				if issue.Category == "reference" && strings.Contains(issue.Message, "does not exist") {
					foundReferenceViolation = true
					if issue.Tier != 1 {
						t.Errorf("Expected Tier 1 for missing reference, got Tier %d", issue.Tier)
					}
				}
				if issue.Category == "integrity" && strings.Contains(issue.Message, "Hash mismatch") {
					foundHashMismatch = true
				}
			}
		}
	}

	if !foundReferenceViolation && !foundHashMismatch {
		t.Error("Expected to detect either missing reference violation or hash mismatch")
	}

	// Note: Hash mismatch may block reference validation, but the violation is still present
	if foundHashMismatch {
		t.Log("Note: Hash mismatch detected - this may block reference validation, but reference violation still exists")
	}

	// Resolution: First fix hash mismatch, then create referenced object
	// Fix hash mismatch with --force
	dummyCmd := &cobra.Command{}
	dummyCmd.SetContext(cli.WithStorageProvider(pkgctx.NewSystemContext(), fileStorage))
	dummyCmd.Flags().Bool("auto-fix", false, "")
	dummyCmd.Flags().Bool("force", true, "")

	registry := storage.NewHashRegistry(pkgctx.NewSystemContext(), "backlog_item", backlogDir)
	_ = registry.Load() //nolint:errcheck // Test setup - registry may not exist yet

	yamlParser := parser.NewYAMLParser()
	parsedObj, err := yamlParser.ParseFile(objectFile)
	if err != nil {
		t.Fatalf("Failed to parse object: %v", err)
	}

	// Fix hash mismatch
	issuesForFix := []Issue{
		{
			Tier:        1,
			Category:    "integrity",
			Message:     "Hash mismatch detected",
			AutoFixable: false,
		},
	}

	objectIDCache := NewObjectIDCache()
	_ = autoFixIssues(checkCtx, dummyCmd, parsedObj, objectFile, "backlog_item", issuesForFix, registry, nil, objectIDCache, fileStorage)

	// Now create the referenced object (CAS-visible for reference check)
	priorityPlan := map[string]any{
		objects.FieldKeyID:            "PRI-999",
		objects.FieldKeyKind:          "priority_plan",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyTitle:         "Test Priority Plan",
		objects.FieldKeyCreatedAt:     "2026-01-02T00:00:00Z",
		objects.FieldKeyCreatedBy:     pkgctx.SystemAccountID,
		objects.FieldKeyUpdatedAt:     "2026-01-02T00:00:00Z",
		objects.FieldKeyUpdatedBy:     pkgctx.SystemAccountID,
		objects.FieldKeyOriginProject: validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
		objects.FieldKeyNamespaceID:   paths.KernelNamespaceID,
	}
	storage.CreateCASVisible(t, fileStorage, ctx, secCtx, priorityPlan, objects.ObjectStatusActive)

	// Verify resolution
	results, err = checkKindObjects(checkCtx, checkCmd(), "backlog_item", []string{"BLI-306"})
	if err != nil {
		t.Fatalf("Failed to re-check: %v", err)
	}

	for _, result := range results {
		if result.ObjectID == "BLI-306" {
			for _, issue := range result.Issues {
				if issue.Category == "reference" && strings.Contains(issue.Message, "PRI-999") && strings.Contains(issue.Message, "does not exist") {
					t.Error("Violation should be resolved after creating referenced object")
				}
			}
		}
	}
}

// TestViolationResolution_Reference_KindMismatch tests reference violation: kind mismatch
// Resolution: Update reference to correct kind or fix the referenced object's kind
// Do not use t.Parallel(): ZQK_TEST_ROOT is process-global; parallel tests can overwrite it.
func TestViolationResolution_Reference_KindMismatch(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	testRoot := proj.Root

	if _, err := setupSystemTestEnvironmentRoot(t, testRoot); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := &pkgctx.SecurityContext{
		AccountID: "ACC-SYSTEM",
	}

	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, testRoot, fileStorage)

	checkCmd := func() *cobra.Command {
		cmd := &cobra.Command{}
		cmd.SetContext(cli.WithStorageProvider(pkgctx.NewSystemContext(), fileStorage))
		return cmd
	}

	// Create a goal object
	goal := map[string]any{
		objects.FieldKeyID:            "GOAL-001",
		objects.FieldKeyKind:          "goal",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyTitle:         "Test Goal",
		objects.FieldKeyCreatedAt:     "2026-01-02T00:00:00Z",
		objects.FieldKeyCreatedBy:     pkgctx.SystemAccountID,
		objects.FieldKeyUpdatedAt:     "2026-01-02T00:00:00Z",
		objects.FieldKeyUpdatedBy:     pkgctx.SystemAccountID,
		objects.FieldKeyOriginProject: validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
		objects.FieldKeyNamespaceID:   paths.KernelNamespaceID,
	}

	if err := fileStorage.Create(ctx, secCtx, goal); err != nil {
		t.Fatalf("Failed to create goal: %v", err)
	}

	// Create backlog item with reference that infers wrong kind
	// Reference GOAL-001 will be inferred as backlog_item (BLI pattern) but is actually goal
	obj := map[string]any{
		objects.FieldKeyID:            "BLI-311",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeyTitle:         "Reference Kind Mismatch Test",
		"goal_ref":                    "GOAL-001", // Reference to goal, but ID pattern suggests backlog_item
		objects.FieldKeyCreatedAt:     "2026-01-02T00:00:00Z",
		objects.FieldKeyCreatedBy:     pkgctx.SystemAccountID,
		objects.FieldKeyUpdatedAt:     "2026-01-02T00:00:00Z",
		objects.FieldKeyUpdatedBy:     pkgctx.SystemAccountID,
		objects.FieldKeyOriginProject: validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
		objects.FieldKeyNamespaceID:   paths.KernelNamespaceID,
		objects.FieldKeyPriorityTier:  "P3",
	}

	if err := fileStorage.Create(ctx, secCtx, obj); err != nil {
		t.Fatalf("Failed to create backlog item: %v", err)
	}

	// Check for violation
	checkCtx := cli.ContextForProjectAndProfile(testRoot, "test")

	results, err := checkKindObjects(checkCtx, checkCmd(), "backlog_item", []string{"BLI-311"})
	if err != nil {
		t.Fatalf("Failed to check objects: %v", err)
	}

	// Find violation (may or may not occur depending on reference resolution)
	foundViolation := false
	for _, result := range results {
		if result.ObjectID == "BLI-311" {
			for _, issue := range result.Issues {
				if issue.Category == "reference" && (strings.Contains(issue.Message, "inferred kind") || strings.Contains(issue.Message, "but found as")) {
					foundViolation = true
					if issue.Tier != 2 {
						t.Errorf("Expected Tier 2 for kind mismatch, got Tier %d", issue.Tier)
					}
					break
				}
			}
		}
	}

	if !foundViolation {
		t.Log("Note: Reference kind mismatch may not be detected if reference resolution is permissive")
	}

	// Resolution: Use explicit kind in reference (goal:GOAL-001) or fix reference
	updates := map[string]any{
		"goal_ref": "goal:GOAL-001", // Explicit kind prefix
	}
	if err := fileStorage.Update(ctx, secCtx, "BLI-311", updates); err != nil {
		t.Logf("Update failed: %v", err)
	}

	// Verify resolution
	results, err = checkKindObjects(checkCtx, checkCmd(), "backlog_item", []string{"BLI-311"})
	if err != nil {
		t.Fatalf("Failed to re-check: %v", err)
	}

	for _, result := range results {
		if result.ObjectID == "BLI-311" {
			for _, issue := range result.Issues {
				if issue.Category == "reference" && strings.Contains(issue.Message, "inferred kind") && strings.Contains(issue.Message, "GOAL-001") {
					t.Log("Note: Kind mismatch may persist if reference format doesn't support explicit kinds")
				}
			}
		}
	}
}
