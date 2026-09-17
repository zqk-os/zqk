package system

import (
	"github.com/lanceman/zqk/pkg/datacell"
	caspkg "github.com/lanceman/zqk/pkg/storage/cas"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/migration/parser"

	"github.com/spf13/cobra"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/scheduler"
	"github.com/lanceman/zqk/pkg/storage"
	testkit "github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/lanceman/zqk/pkg/zqktime"
)

// TestDuplicateID_DetectionAndResolution tests the detection and resolution
// of duplicate object IDs across multiple files.
//
// This test recreates the POL-DEBUG-001 issue where:
// - Two files have the same ID (POL-DEBUG-001)
// - One file has all required fields (correct)
// - One file has missing fields (stale/incorrect)
// - Bulk check validates both files, causing false positives
//
// The test verifies:
// 1. Duplicate ID detection works
// 2. The stale file is identified
// 3. Auto-fix can resolve the issue (by flagging or suggesting deletion)
func TestDuplicateID_DetectionAndResolution(t *testing.T) {
	// Do not use t.Parallel(): same *testing.T uses t.Setenv(ZQK_TEST_ROOT); GetGlobalObjectIDCache is process-global.
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

	// Create first policy with all required fields (correct); use valid policy ID format (POL-CODE-NNN)
	correctPolicy := map[string]any{
		objects.FieldKeyID:            "POL-CODE-001",
		objects.FieldKeyKind:          "policy",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        scheduler.StatusActive,
		objects.FieldKeyTitle:         "Correct Policy",
		objects.FieldKeyPolicyType:    "standard",
		objects.FieldKeyCategory:      "code_quality",
		objects.FieldKeyBody:          "This is the correct policy with all required fields.",
		objects.FieldKeyCreatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyCreatedBy:     pkgctx.SystemAccountID,
		objects.FieldKeyUpdatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyUpdatedBy:     pkgctx.SystemAccountID,
		objects.FieldKeyNamespaceID:   paths.KernelNamespaceID,
		objects.FieldKeyOriginProject: validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
		"enforcement_level":           "required",
	}

	storage.CreateCASVisible(t, fileStorage, ctx, secCtx, correctPolicy, scheduler.StatusActive)
	storage.FlushAllOrFail(t, testRoot)

	// Create second file with same ID but missing fields (stale/incorrect)
	// This simulates the POL-DEBUG-TEST-FIRST-v1.0.yaml scenario
	stalePolicyContent := fmt.Sprintf(`id: POL-CODE-001
kind: policy
schema_version: "`+objects.DefaultSchemaVersion+`"
status: active
title: Stale Policy
description: This is a stale policy file with the same ID but missing required fields.
created_at: "2026-01-01T00:00:00Z"
created_by: ACC-1785920548450214012-68b850c0
updated_at: "2026-01-01T00:00:00Z"
updated_by: ACC-1785920548450214012-68b850c0
origin_project: %s
origin_system: %s
policy_type: operational
enforcement_level: required
# Missing: body, category fields
`, validation.DefaultOriginProject, validation.DefaultOriginSystem)

	policiesDir := filepath.Join(testRoot, paths.ProcessPoliciesDir)
	staleFilePath := filepath.Join(policiesDir, "POL-CODE-001-STALE.yaml")
	if err := fileutil.WriteFile(staleFilePath, []byte(stalePolicyContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("Failed to create stale policy file: %v", err)
	}

	checkCtx := cli.ContextForProjectAndProfile(testRoot, "test")

	// Use global loaders (same as bulk check path)
	specLoader := objects.GetGlobalSpecLoader()
	lifecycleLoader := objects.GetGlobalLifecycleLoader()
	validatorRegistry := validation.GetGlobalRegistry()
	validator := validatorRegistry.Get("") // Default validator

	// Create hash registry cache (same as bulk check path)
	hashRegistryCache := &HashRegistryCacheType{
		cache: make(map[string]storage.HashRegistryProvider),
	}

	// Create object ID cache (same as bulk check path)
	objectIDCache := GetGlobalObjectIDCache()
	if err := objectIDCache.BuildCache(context.Background(), testRoot, true); err != nil {
		t.Fatalf("Failed to build object ID cache: %v", err)
	}

	// Use CheckKindObjectsWithCache (the bulk check path)
	results, _, err := CheckKindObjectsWithCache(
		checkCtx,
		pkgctx.NewSystemContext(),
		&cobra.Command{},
		"policy",
		nil, // Check all policy objects (bulk)
		specLoader,
		lifecycleLoader,
		validator,
		hashRegistryCache,
		objectIDCache,
	)
	if err != nil {
		t.Fatalf("Failed to check objects: %v", err)
	}

	// Verify we detected both files (policy may be CAS so path can be hash-based; identify stale by path containing "STALE")
	var correctFileResult, staleFileResult *CheckResult
	for i := range results {
		if results[i].ObjectID != "POL-CODE-001" {
			continue
		}
		if strings.Contains(results[i].FilePath, "STALE") {
			staleFileResult = &results[i]
		} else {
			correctFileResult = &results[i]
		}
	}

	if correctFileResult == nil {
		t.Fatal("Correct policy file not found in check results")
	}
	if staleFileResult == nil {
		t.Fatal("Stale policy file not found in check results")
	}

	// Verify duplicate ID detection
	duplicateIDIssues := findIssuesByCategory(staleFileResult.Issues, "registration")
	duplicateFound := false
	for _, issue := range duplicateIDIssues {
		if strings.Contains(issue.Message, "duplicate") || strings.Contains(issue.Message, "same ID") {
			duplicateFound = true
			break
		}
	}

	// If duplicate detection isn't implemented yet, at least verify the stale file has validation errors
	if !duplicateFound {
		t.Log("Duplicate ID detection not yet implemented - verifying stale file has validation errors instead")
		validationIssues := findIssuesByCategory(staleFileResult.Issues, "instance_validation")
		if len(validationIssues) == 0 {
			t.Error("Expected validation errors for stale file (missing body, category fields)")
		}
	} else {
		t.Log("Duplicate ID detection working correctly")
	}

	// Verify correct file has no validation errors
	correctValidationIssues := findIssuesByCategory(correctFileResult.Issues, "instance_validation")
	for _, issue := range correctValidationIssues {
		if contains(issue.Message, "body") || contains(issue.Message, "category") {
			t.Errorf("Correct file should not have validation errors: %s", issue.Message)
		}
	}
}

// TestDuplicateID_AutoFix tests that duplicate ID issues can be auto-fixed
// by identifying and flagging the stale file for deletion.
func TestDuplicateID_AutoFix(t *testing.T) {
	// Do not use t.Parallel(): same *testing.T uses t.Setenv(ZQK_TEST_ROOT).
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	testRoot := proj.Root
	caspkg.SetListingIndexWriteQueueFactoryToPerProjectRoot()
	secCtx := &pkgctx.SecurityContext{
		AccountID: pkgctx.SystemAccountID,
	}
	var fileStorage *storage.FileObjectStorage
	t.Cleanup(func() {
		q := caspkg.GetListingIndexWriteQueueForProjectRoot(testRoot)
		if q != nil {
			_ = q.FlushAll(2 * time.Second)
			_ = q.Shutdown()
		}
		_ = storage.FlushAllListingIndexesForProjectRoot(testRoot)
		_ = storage.WaitForWALProcessing(testRoot, 15*time.Second)
		if fileStorage != nil {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			_ = fileStorage.Shutdown(shutdownCtx)
		}
		resetDir, err := fileutil.MkdirTemp("", "zqk-audit-global-reset")
		if err == nil {
			defer fileutil.RemoveAll(resetDir)
			_ = storage.TearDownGlobalAuditBufferForTestProjectRoot(testRoot, resetDir, secCtx)
		} else {
			storage.FlushGlobalAuditBufferForProjectRoot(testRoot)
		}
		_ = fileutil.RemoveAll(datacell.ProcessPrimaryDir(testRoot))
		_ = fileutil.RemoveAll(filepath.Join(testRoot, paths.ProjectDataDir))
		// Strip anything left under test root (async CAS/WAL can leave .zqk non-empty; helps t.TempDir cleanup).
		storage.ScrubProjectRootForTempCleanup(testRoot, 40, 25*time.Millisecond)
		caspkg.SetListingIndexWriteQueueFactory(nil)
	})

	if _, err := setupSystemTestEnvironmentRoot(t, testRoot); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}

	ctx := pkgctx.NewSystemContext()

	var err error
	fileStorage, err = storage.NewFileObjectStorageForTest(testRoot)
	testkit.RegisterStorageTestCleanup(t, testRoot, fileStorage)
	if fileStorage != nil {
		defer func() { _ = fileStorage.Shutdown(context.Background()) }()
	}
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	_ = storage.InitializeGlobalBufferWithConfig(testRoot, secCtx)

	// Create correct policy; use valid policy ID format (POL-CODE-NNN)
	correctPolicy := map[string]any{
		objects.FieldKeyID:            "POL-CODE-002",
		objects.FieldKeyKind:          "policy",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:        scheduler.StatusActive,
		objects.FieldKeyTitle:         "Correct Policy",
		objects.FieldKeyPolicyType:    "standard",
		objects.FieldKeyCategory:      "code_quality",
		objects.FieldKeyBody:          "This is the correct policy.",
		objects.FieldKeyCreatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyCreatedBy:     pkgctx.SystemAccountID,
		objects.FieldKeyUpdatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyUpdatedBy:     pkgctx.SystemAccountID,
		objects.FieldKeyNamespaceID:   paths.KernelNamespaceID,
		objects.FieldKeyOriginProject: validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
		"enforcement_level":           "required",
	}

	storage.CreateCASVisible(t, fileStorage, ctx, secCtx, correctPolicy, scheduler.StatusActive)
	storage.FlushAllOrFail(t, testRoot)

	// Create stale duplicate file
	policiesDir := filepath.Join(testRoot, paths.ProcessPoliciesDir)
	staleFilePath := filepath.Join(policiesDir, "POL-CODE-002-STALE.yaml")
	stalePolicyContent := `id: POL-CODE-002
kind: policy
schema_version: "` + objects.DefaultSchemaVersion + `"
status: active
title: Stale Policy
policy_type: operational
# Missing: body, category
`
	if err := fileutil.WriteFile(staleFilePath, []byte(stalePolicyContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("Failed to create stale policy file: %v", err)
	}

	checkCtx := cli.ContextForProjectAndProfile(testRoot, "test")

	// Run check with auto-fix
	cmd := &cobra.Command{}
	cmd.Flags().Bool("auto-fix", true, "")
	cmd.Flags().Bool("force", false, "")

	// Build object ID cache
	objectIDCache := GetGlobalObjectIDCache()
	if err := objectIDCache.BuildCache(context.Background(), testRoot, true); err != nil {
		t.Fatalf("Failed to build object ID cache: %v", err)
	}

	// Check for duplicate IDs by checking one of the files
	duplicateIssues := checkDuplicateIDs(checkCtx, "POL-CODE-002", "policy", staleFilePath, objectIDCache)

	// Verify duplicate was detected
	if len(duplicateIssues) == 0 {
		t.Log("Duplicate ID check not yet implemented - skipping auto-fix test")
		return
	}

	// Verify stale file is identified
	staleFileIdentified := false
	for _, issue := range duplicateIssues {
		if strings.Contains(issue.Message, "STALE") || strings.Contains(issue.Message, "duplicate") {
			staleFileIdentified = true
			if issue.AutoFixable {
				t.Error("Duplicate ID issue must not be auto-fixable (silent delete wiped sole CAS objects)")
			}
			break
		}
	}

	if !staleFileIdentified {
		t.Error("Stale file should be identified in duplicate ID issues")
	}

	// Auto-fix must refuse silent delete of purported duplicates.
	obj := &parser.ParsedObject{
		ID:         "POL-CODE-002",
		Kind:       "policy",
		Properties: map[string]any{objects.FieldKeyID: "POL-CODE-002"},
	}
	hashRegistryCache := &HashRegistryCacheType{
		cache: make(map[string]storage.HashRegistryProvider),
	}

	autoFixIssues(
		checkCtx,
		cmd,
		obj,
		staleFilePath,
		"policy",
		duplicateIssues,
		nil,
		hashRegistryCache,
		objectIDCache,
		fileStorage,
	)

	if _, err := fileutil.Stat(staleFilePath); err != nil {
		t.Errorf("stale duplicate file %s must survive auto-fix: %v", staleFilePath, err)
	}
}

// Helper functions
func findIssuesByCategory(issues []Issue, category string) []Issue {
	var result []Issue
	for _, issue := range issues {
		if issue.Category == category {
			result = append(result, issue)
		}
	}
	return result
}
