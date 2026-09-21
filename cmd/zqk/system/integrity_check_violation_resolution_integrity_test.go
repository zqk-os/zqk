package system

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	"github.com/zqk-os/zqk/internal/cli"

	"github.com/spf13/cobra"

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

// TestViolationResolution_Integrity_MissingHash tests integrity violation: missing hash (already covered in other tests)
// This test verifies the resolution path is straightforward
// Do not use t.Parallel(): ZQK_TEST_ROOT is process-global.
func TestViolationResolution_Integrity_MissingHash(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	testRoot := proj.Root
	caspkg.SetListingIndexWriteQueueFactoryToPerProjectRoot()
	secCtx := &pkgctx.SecurityContext{AccountID: pkgctx.SystemAccountID}
	t.Cleanup(func() {
		resetDir, err := fileutil.MkdirTemp("", "zqk-audit-global-reset")
		opts := testkit.TeardownOptions{
			ProjectRoot:           testRoot,
			StripProcessArtifacts: true,
			WALTimeout:            20 * time.Second,
			ShutdownTimeout:       20 * time.Second,
		}
		if err == nil {
			defer fileutil.RemoveAll(resetDir)
			opts.TearDownGlobalAuditBuffer = true
			opts.SecCtx = secCtx
			opts.AuditBufferResetRoot = resetDir
		}
		_ = testkit.RunStandardTeardown(opts)
		caspkg.SetListingIndexWriteQueueFactory(nil)
	})

	if _, err := setupSystemTestEnvironmentRoot(t, testRoot); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}

	backlogDir := datacell.CellCASPrimaryDir(testRoot, objects.GetDirectoryFromKind(objects.KindBacklogItem))
	if err := fileutil.MkdirAll(backlogDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create backlog directory: %v", err)
	}

	// Create file outside CLI (missing hash)
	objectFile := filepath.Join(backlogDir, "BLI-308.yaml")
	objectContent := `id: BLI-308
kind: backlog_item
schema_version: "` + objects.DefaultSchemaVersion + `"
status: exploring
title: Missing Hash Test
created_at: "2026-01-02T00:00:00Z"
created_by: ACC-SYSTEM
updated_at: "2026-01-02T00:00:00Z"
updated_by: ACC-SYSTEM
origin_project: zqk
origin_system: zqk
namespace_id: zqk:kernel
priority_tier: P3
`

	if err := fileutil.WriteFile(objectFile, []byte(objectContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("Failed to create object file: %v", err)
	}

	ctx := cli.ContextForProjectAndProfile(testRoot, "test")

	// Check for violation
	yamlParser := parser.NewYAMLParser()
	parsedObj, err := yamlParser.ParseFile(objectFile)
	if err != nil {
		t.Fatalf("Failed to parse object: %v", err)
	}

	cmd := &cobra.Command{}
	systemCtx := pkgctx.NewSystemContext()
	cmd.SetContext(systemCtx)
	issues, _ := checkIntegrity(ctx, cmd, parsedObj, objectFile, "backlog_item")

	// Find violation
	foundViolation := false
	for _, issue := range issues {
		if issue.Category == "integrity" && strings.Contains(issue.Message, "No integrity hash recorded") {
			foundViolation = true
			if !issue.AutoFixable {
				t.Error("Expected missing hash to be auto-fixable")
			}
			break
		}
	}

	if !foundViolation {
		t.Error("Expected to detect missing hash violation")
	}

	// Resolution: Use --auto-fix flag
	dummyCmd := &cobra.Command{}
	dummyCmd.Flags().Bool("auto-fix", true, "")
	dummyCmd.Flags().Bool("force", false, "")
	dummyCmd.SetContext(systemCtx)

	registry := storage.NewHashRegistry(pkgctx.NewSystemContext(), "backlog_item", backlogDir)
	_ = registry.Load() //nolint:errcheck // Test setup - registry may not exist yet - registry may not exist yet

	issuesForFix := []Issue{
		{
			Tier:        3,
			Category:    "integrity",
			Message:     "No integrity hash recorded for this file",
			AutoFixable: true,
		},
	}

	objectIDCache := NewObjectIDCache()
	fixed := autoFixIssues(ctx, dummyCmd, parsedObj, objectFile, "backlog_item", issuesForFix, registry, nil, objectIDCache, nil)

	if len(fixed) == 0 {
		t.Error("Expected auto-fix to resolve missing hash violation")
	}

	// Verify resolution (cmd already declared earlier in function)
	issues, _ = checkIntegrity(ctx, cmd, parsedObj, objectFile, "backlog_item")
	for _, issue := range issues {
		if issue.Category == "integrity" && strings.Contains(issue.Message, "No integrity hash recorded") {
			t.Error("Violation should be resolved after auto-fix")
		}
	}
}

// TestViolationResolution_Integrity_HashMismatch tests integrity violation: hash mismatch
// Resolution: Use --force flag to regenerate hash
// Do not use t.Parallel(): ZQK_TEST_ROOT is process-global; parallel tests can overwrite it.
func TestViolationResolution_Integrity_HashMismatch(t *testing.T) {
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

	// Create + promote off draft plane (CAS-visible) so hash registry / --force CAS fix share one plane.
	// draft-plane create / promote membrane.
	obj := map[string]any{
		objects.FieldKeyID:            "BLI-309",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeyTitle:         "Hash Mismatch Test",
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

	// Resolve path after Create (CAS uses hash-named files; use storage's process dir so we scan where it wrote)
	backlogDir := datacell.CellCASPrimaryDir(testRoot, objects.GetDirectoryFromKind(objects.KindBacklogItem))
	objectFile, err := fileStorage.GetFilePathForObject("BLI-309", "backlog_item")
	if err != nil {
		// Fallback: scan kind dir for hash-named file containing id: BLI-309 (index may not be flushed yet)
		objectFile, err = resolveBacklogItemPathByScan(backlogDir, "BLI-309")
		if err != nil {
			t.Fatalf("Failed to resolve path for BLI-309 after create: %v", err)
		}
	}

	// Modify file directly (creates hash mismatch)
	content, err := fileutil.ReadFile(objectFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	modifiedContent := string(content) + "\n# Modified outside CLI\n"
	if err := fileutil.WriteFile(objectFile, []byte(modifiedContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("Failed to modify file: %v", err)
	}

	// Check for violation (use checkIntegrityWithRegistryAndContent so CAS hash-named file gets content-vs-registry mismatch check)
	checkCtx := cli.ContextForProjectAndProfile(testRoot, "test")

	yamlParser := parser.NewYAMLParser()
	parsedObj, err := yamlParser.ParseFile(objectFile)
	if err != nil {
		t.Fatalf("Failed to parse object: %v", err)
	}

	cmd := &cobra.Command{}
	systemCtx := pkgctx.NewSystemContext()
	cmd.SetContext(systemCtx)
	registry := storage.NewHashRegistry(pkgctx.NewSystemContext(), "backlog_item", backlogDir)
	_ = registry.Load() //nolint:errcheck
	issues, _ := checkIntegrityWithRegistryAndContent(checkCtx, parsedObj, objectFile, "backlog_item", []byte(modifiedContent), registry, nil)

	// Find violation (hash mismatch, or CAS index missing when content was modified)
	foundViolation := false
	for _, issue := range issues {
		if issue.Category != "integrity" {
			continue
		}
		if strings.Contains(issue.Message, "Hash mismatch detected") {
			foundViolation = true
			if issue.Tier != 1 {
				t.Errorf("Expected Tier 1 for hash mismatch, got Tier %d", issue.Tier)
			}
			// CAS path may mark as auto-fixable; --force is still required for audit
			break
		}
		if strings.Contains(issue.Message, "object not in CAS index") || strings.Contains(issue.Message, "CAS index out of sync") {
			// Index may not be flushed yet after Create; still indicates integrity issue
			foundViolation = true
			break
		}
	}

	if !foundViolation {
		t.Error("Expected to detect hash mismatch or CAS index integrity violation")
	}

	// Resolution: Use --force flag (pass fileStorage so fixCASIndexOutOfSync hits the same CAS index)
	dummyCmd := &cobra.Command{}
	dummyCmd.Flags().Bool("auto-fix", false, "")
	dummyCmd.Flags().Bool("force", true, "")
	dummyCmd.SetContext(cli.WithStorageProvider(systemCtx, fileStorage))

	issuesForFix := []Issue{
		{
			Tier:        1,
			Category:    "integrity",
			Message:     "Hash mismatch detected",
			AutoFixable: false,
		},
	}

	objectIDCache := NewObjectIDCache()
	fixed := autoFixIssues(checkCtx, dummyCmd, parsedObj, objectFile, "backlog_item", issuesForFix, registry, nil, objectIDCache, fileStorage)

	if len(fixed) == 0 {
		t.Error("Expected --force to resolve hash mismatch violation")
	}

	// Verify resolution (cmd already declared earlier in function)
	issues, _ = checkIntegrity(checkCtx, cmd, parsedObj, objectFile, "backlog_item")
	for _, issue := range issues {
		if issue.Category == "integrity" && strings.Contains(issue.Message, "Hash mismatch detected") {
			t.Error("Violation should be resolved after --force fix")
		}
	}
}
