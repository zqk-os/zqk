package system

import (
	"crypto/sha256"
	"encoding/hex"
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
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/objects"
)

// TestCacheKeyConsistency_BucketedObjects_AutoFixVisible verifies that after auto-fix
// updates a hash registry for a bucketed object, subsequent checks can see the update.
//
// NOTE: This test is currently skipped because all kinds now use CAS (Content-Addressable Storage)
// instead of HashRegistry. The test needs to be updated to test CAS index consistency instead.
//
// This test prevents regression of the bug where:
// - Auto-fix uses cache key: "change_journal_entry:.zqk/process/change_journal/2026-01/"
// - Async validation used cache key: "change_journal_entry" (WRONG)
// - Result: Auto-fix updates registry in subdirectory, but validation loads from kind directory
// - Same violations detected repeatedly, causing infinite loop
//
// The test verifies that:
// 1. Auto-fix updates hash in subdirectory registry
// 2. Subsequent check loads from subdirectory registry (same cache key)
// 3. No violation is detected (cache key consistency)
func TestCacheKeyConsistency_BucketedObjects_AutoFixVisible(t *testing.T) {
	// Not t.Parallel: same *testing.T uses t.Setenv(ZQK_TEST_ROOT) (same if this test is un-skipped).
	t.Skip("Skipping: All kinds now use CAS instead of HashRegistry. Test needs update to test CAS index consistency.")
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	testRoot := proj.Root

	if _, err := setupSystemTestEnvironmentRoot(t, testRoot); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}

	// Create a change_journal_entry file directly in a monthly subdirectory
	// This simulates a file created outside CLI (missing hash)
	now := time.Now()
	monthDir := now.Format("2006-01")
	kindDir := datacell.CellCASPrimaryDir(testRoot, "change_journal")
	subDir := filepath.Join(kindDir, monthDir)
	if err := fileutil.MkdirAll(subDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	filePath := filepath.Join(subDir, "CHA-9001.yaml")
	fileContent := `id: CHA-9001
kind: change_journal_entry
schema_version: "` + objects.DefaultSchemaVersion + `"
status: completed
change_type: create
object_ref: backlog_item:BLI-900
title: Test Change Journal Entry
created_at: "2026-01-02T15:00:00Z"
created_by: ACC-1785920548450214012-68b850c0
updated_at: "2026-01-02T15:00:00Z"
updated_by: ACC-1785920548450214012-68b850c0
namespace_id: zqk:kernel
origin_project: zqk
origin_system: zqk
`

	if err := fileutil.WriteFile(filePath, []byte(fileContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("Failed to create file: %v", err)
	}

	checkCtx := cli.ContextForProjectAndProfile(testRoot, "test")

	// Step 1: Initial check - should detect missing hash
	results, err := checkKindObjects(checkCtx, &cobra.Command{}, "change_journal_entry", []string{"CHA-9001"})
	if err != nil {
		t.Fatalf("Failed to check objects: %v", err)
	}

	// Verify missing hash is detected
	foundMissingHash := false
	for _, result := range results {
		if result.ObjectID == "CHA-9001" {
			for _, issue := range result.Issues {
				if issue.Category == "integrity" && issue.AutoFixable && strings.Contains(issue.Message, "No integrity hash recorded") {
					foundMissingHash = true
					break
				}
			}
		}
	}
	if !foundMissingHash {
		t.Error("Expected to detect missing integrity hash")
	}

	// Step 2: Run auto-fix - should update hash in subdirectory registry
	cmd := &cobra.Command{}
	cmd.Flags().Bool("auto-fix", false, "")
	cmd.Flags().Bool("force", false, "")
	_ = cmd.Flags().Set("auto-fix", "true") //nolint:errcheck // Test setup - flag set errors are acceptable
	_ = cmd.Flags().Set("force", "true")    //nolint:errcheck // Test setup - flag set errors are acceptable

	results, err = checkKindObjects(checkCtx, cmd, "change_journal_entry", []string{"CHA-9001"})
	if err != nil {
		t.Fatalf("Failed to check with auto-fix: %v", err)
	}

	// Verify auto-fix was applied
	foundAutoFixed := false
	for _, result := range results {
		if result.ObjectID == "CHA-9001" {
			if len(result.AutoFixed) > 0 {
				foundAutoFixed = true
			}
		}
	}
	if !foundAutoFixed {
		t.Error("Expected auto-fix to update hash")
	}

	// Step 3: Verify hash registry exists in subdirectory
	// Note: HashRegistry.Save() uses async worker, so we may need to wait briefly
	subDirRegistryPath := filepath.Join(subDir, ".change_journal_entry.hashes")
	maxWait := 5 * time.Second
	waitInterval := 50 * time.Millisecond
	waited := time.Duration(0)
	for {
		if _, err := fileutil.Stat(subDirRegistryPath); err == nil {
			break // File exists
		}
		if waited >= maxWait {
			t.Errorf("Expected hash registry in subdirectory %s, got error: %v (waited %v)", subDir, err, waited)
			break
		}
		time.Sleep(waitInterval)
		waited += waitInterval
	}

	// Step 4: Calculate expected hash and verify it's in the registry
	expectedHash := sha256.Sum256([]byte(fileContent))
	expectedHashStr := hex.EncodeToString(expectedHash[:])

	subDirRegistry := storage.NewHashRegistry(pkgctx.NewSystemContext(), "change_journal_entry", subDir)
	if err := subDirRegistry.Load(); err != nil {
		t.Fatalf("Failed to load registry from subdirectory: %v", err)
	}

	actualHash := subDirRegistry.GetHash("CHA-9001.yaml")
	if actualHash != expectedHashStr {
		expectedPrefix := expectedHashStr
		actualPrefix := actualHash
		if len(expectedHashStr) >= 16 {
			expectedPrefix = expectedHashStr[:16]
		}
		if len(actualHash) >= 16 {
			actualPrefix = actualHash[:16]
		}
		t.Errorf("Expected hash %s, got %s", expectedPrefix, actualPrefix)
	}

	// Step 5: Re-check - should NOT detect missing hash (cache key consistency)
	// This is the critical test - if cache keys don't match, this will fail
	// because validation will load from kind directory (wrong location) and won't see the hash
	results, err = checkKindObjects(checkCtx, &cobra.Command{}, "change_journal_entry", []string{"CHA-9001"})
	if err != nil {
		t.Fatalf("Failed to re-check objects: %v", err)
	}

	// Verify no missing hash violation (should be fixed)
	// This is the core test - if cache keys don't match, validation will load
	// from kind directory and won't see the hash in subdirectory
	for _, result := range results {
		if result.ObjectID == "CHA-9001" {
			for _, issue := range result.Issues {
				if issue.Category == "integrity" && strings.Contains(issue.Message, "No integrity hash recorded") {
					t.Errorf("Hash violation should be resolved after auto-fix (cache key consistency), but got: %s", issue.Message)
				}
			}
		}
	}
}

// TestCacheKeyConsistency_AsyncValidationPath verifies that the async validation
// path uses the same cache key format as auto-fix for bucketed objects.
//
// This test directly tests the cache key logic in async_check.go to ensure
// it matches the cache key format used in check_impl.go (auto-fix).
func TestCacheKeyConsistency_AsyncValidationPath(t *testing.T) {
	// Do not use t.Parallel(): same *testing.T uses t.Setenv(ZQK_TEST_ROOT).
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	testRoot := proj.Root

	if _, err := setupSystemTestEnvironmentRoot(t, testRoot); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}

	// Create a change_journal_entry file in a monthly subdirectory
	now := time.Now()
	monthDir := now.Format("2006-01")
	kindDir := datacell.CellCASPrimaryDir(testRoot, "change_journal")
	subDir := filepath.Join(kindDir, monthDir)
	if err := fileutil.MkdirAll(subDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	filePath := filepath.Join(subDir, "CHA-9002.yaml")
	fileContent := `id: CHA-9002
kind: change_journal_entry
schema_version: "` + objects.DefaultSchemaVersion + `"
status: completed
change_type: create
object_ref: backlog_item:BLI-900
title: Test Change Journal Entry 2
created_at: "2026-01-02T15:00:00Z"
created_by: ACC-1785920548450214012-68b850c0
updated_at: "2026-01-02T15:00:00Z"
updated_by: ACC-1785920548450214012-68b850c0
namespace_id: zqk:kernel
origin_project: zqk
origin_system: zqk
`

	if err := fileutil.WriteFile(filePath, []byte(fileContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("Failed to create file: %v", err)
	}

	// Create hash registry in subdirectory (simulating auto-fix)
	hash := sha256.Sum256([]byte(fileContent))
	hashStr := hex.EncodeToString(hash[:])

	subDirRegistry := storage.NewHashRegistry(pkgctx.NewSystemContext(), "change_journal_entry", subDir)
	subDirRegistry.SetHash("CHA-9002.yaml", hashStr)
	if err := subDirRegistry.Save(); err != nil {
		t.Fatalf("Failed to save registry: %v", err)
	}

	// Verify registry is in subdirectory
	registryPath := filepath.Join(subDir, ".change_journal_entry.hashes")
	if _, err := fileutil.Stat(registryPath); err != nil {
		t.Fatalf("Registry should exist in subdirectory: %v", err)
	}

	// Now simulate what async validation does:
	// 1. Parse the file (verify it's valid)
	yamlParser := parser.NewYAMLParser()
	_, err := yamlParser.ParseFile(filePath)
	if err != nil {
		t.Fatalf("Failed to parse file: %v", err)
	}

	// 2. Determine if bucketed (fileDir != kindDir)
	fileDir := filepath.Dir(filePath)
	isBucketed := fileDir != kindDir
	if !isBucketed {
		t.Fatal("Expected object to be bucketed (fileDir != kindDir)")
	}

	// 3. Build cache key (should match auto-fix format)
	var cacheKey string
	if isBucketed {
		cacheKey = "change_journal_entry:" + fileDir
	} else {
		cacheKey = "change_journal_entry"
	}

	expectedCacheKey := "change_journal_entry:" + subDir
	if cacheKey != expectedCacheKey {
		t.Errorf("Cache key mismatch: got %s, expected %s", cacheKey, expectedCacheKey)
	}

	// 4. Load registry from file's directory (not kind directory)
	registryDir := fileDir
	if !isBucketed {
		registryDir = kindDir
	}

	if registryDir != subDir {
		t.Errorf("Registry directory mismatch: got %s, expected %s", registryDir, subDir)
	}

	// 5. Verify hash can be retrieved
	testRegistry := storage.NewHashRegistry(pkgctx.NewSystemContext(), "change_journal_entry", registryDir)
	if err := testRegistry.Load(); err != nil {
		t.Fatalf("Failed to load registry from %s: %v", registryDir, err)
	}

	retrievedHash := testRegistry.GetHash("CHA-9002.yaml")
	if retrievedHash != hashStr {
		t.Errorf("Hash mismatch: got %s, expected %s", retrievedHash[:16], hashStr[:16])
	}

	// 6. Verify kind directory registry (if it exists) doesn't have the hash
	kindDirRegistryPath := filepath.Join(kindDir, ".change_journal_entry.hashes")
	if _, err := fileutil.Stat(kindDirRegistryPath); err == nil {
		kindDirRegistry := storage.NewHashRegistry(pkgctx.NewSystemContext(), "change_journal_entry", kindDir)
		if err := kindDirRegistry.Load(); err == nil {
			kindHash := kindDirRegistry.GetHash("CHA-9002.yaml")
			if kindHash != emptyValue {
				t.Errorf("Hash should NOT be in kind directory registry for bucketed objects, but found: %s", kindHash)
			}
		}
	}
}

// TestCacheKeyConsistency_NonBucketedObjects verifies that non-bucketed objects
// use the same cache key format in both async validation and auto-fix.
func TestCacheKeyConsistency_NonBucketedObjects(t *testing.T) {
	// Not t.Parallel: teardown must drain .zqk and .zqk/process before t.TempDir cleanup.
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	testRoot := proj.Root

	if _, err := setupSystemTestEnvironmentRoot(t, testRoot); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}

	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	t.Cleanup(func() {
		resetDir, rerr := fileutil.MkdirTemp("", "zqk-audit-global-reset")
		if rerr != nil {
			// Fallback: still drain CAS queue + strip artifacts, but skip global audit buffer teardown.
			if q := caspkg.GetGlobalListingIndexWriteQueue(); q != nil {
				_ = q.FlushAll(5 * time.Second) //nolint:errcheck // best-effort test cleanup
			}
			_ = testkit.RunStandardTeardown(testkit.TeardownOptions{
				ProjectRoot:           testRoot,
				FileStorage:           fileStorage,
				StripProcessArtifacts: true,
				WALTimeout:            30 * time.Second,
				ShutdownTimeout:       30 * time.Second,
			})
			return
		}
		defer fileutil.RemoveAll(resetDir)

		secCtxAlready := &pkgctx.SecurityContext{AccountID: pkgctx.SystemAccountID}
		// Ensure we drain global CAS queue before teardown (best-effort; test storage may also enqueue work).
		if q := caspkg.GetGlobalListingIndexWriteQueue(); q != nil {
			_ = q.FlushAll(5 * time.Second) //nolint:errcheck // best-effort test cleanup
		}

		_ = testkit.RunStandardTeardown(testkit.TeardownOptions{
			ProjectRoot:               testRoot,
			FileStorage:               fileStorage,
			StripProcessArtifacts:     true,
			WALTimeout:                30 * time.Second,
			ShutdownTimeout:           30 * time.Second,
			TearDownGlobalAuditBuffer: true,
			SecCtx:                    secCtxAlready,
			AuditBufferResetRoot:      resetDir,
		})
	})

	// Create a policy file directly in kind directory (non-bucketed)
	kindDir := filepath.Join(testRoot, paths.ProcessPoliciesDir)
	if err := fileutil.MkdirAll(kindDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create kind directory: %v", err)
	}

	filePath := filepath.Join(kindDir, "POL-9001.yaml")
	fileContent := `id: POL-9001
kind: policy
schema_version: "` + objects.DefaultSchemaVersion + `"
status: active
policy_type: standard
category: code_quality
body: Test policy body
created_at: "2026-01-02T15:00:00Z"
created_by: ACC-1785920548450214012-68b850c0
updated_at: "2026-01-02T15:00:00Z"
updated_by: ACC-1785920548450214012-68b850c0
namespace_id: zqk:kernel
origin_project: zqk
origin_system: zqk
enforcement_level: required
`

	if err := fileutil.WriteFile(filePath, []byte(fileContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("Failed to create file: %v", err)
	}

	checkCtx := cli.ContextForProjectAndProfile(testRoot, "test")

	// Step 1: Initial check - should detect missing hash
	results, err := checkKindObjects(checkCtx, &cobra.Command{}, "policy", []string{"POL-9001"})
	if err != nil {
		t.Fatalf("Failed to check objects: %v", err)
	}

	// Verify missing hash is detected
	foundMissingHash := false
	for _, result := range results {
		if result.ObjectID == "POL-9001" {
			for _, issue := range result.Issues {
				if issue.Category == "integrity" && issue.AutoFixable && strings.Contains(issue.Message, "No integrity hash recorded") {
					foundMissingHash = true
					break
				}
			}
		}
	}
	if !foundMissingHash {
		t.Error("Expected to detect missing integrity hash")
	}

	// Step 2: Run auto-fix - should update hash in kind directory registry
	cmd := &cobra.Command{}
	cmd.Flags().Bool("auto-fix", false, "")
	cmd.Flags().Bool("force", false, "")
	_ = cmd.Flags().Set("auto-fix", "true") //nolint:errcheck // Test setup - flag set errors are acceptable
	_ = cmd.Flags().Set("force", "true")    //nolint:errcheck // Test setup - flag set errors are acceptable

	results, err = checkKindObjects(checkCtx, cmd, "policy", []string{"POL-9001"})
	if err != nil {
		t.Fatalf("Failed to check with auto-fix: %v", err)
	}

	// Verify auto-fix was applied
	foundAutoFixed := false
	for _, result := range results {
		if result.ObjectID == "POL-9001" {
			if len(result.AutoFixed) > 0 {
				foundAutoFixed = true
			}
		}
	}
	if !foundAutoFixed {
		t.Error("Expected auto-fix to update hash")
	}

	// Step 3: Verify CAS index exists (all kinds now use CAS instead of HashRegistry)
	// CAS index is stored in .zqk/system-health/cas-index/policy.json
	// Skip hash registry check - CAS migration means hash registries no longer exist
	// The test verifies cache key consistency, which is now handled by CAS index

	// Step 4: Re-check - should NOT detect missing hash (cache key consistency)
	results, err = checkKindObjects(checkCtx, &cobra.Command{}, "policy", []string{"POL-9001"})
	if err != nil {
		t.Fatalf("Failed to re-check objects: %v", err)
	}

	// Verify no missing hash violation (should be fixed)
	for _, result := range results {
		if result.ObjectID == "POL-9001" {
			for _, issue := range result.Issues {
				if issue.Category == "integrity" && strings.Contains(issue.Message, "No integrity hash recorded") {
					t.Errorf("Hash violation should be resolved after auto-fix, but got: %s", issue.Message)
				}
			}
		}
	}
}
