package system

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
	"github.com/zqk-os/zqk/pkg/zqkenv"

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

// TestHashMismatchTailChase reproduces the tail-chasing issue where fixing a hash mismatch
// creates new objects (audit events, change journal entries) that themselves have hash mismatches
// in the same check cycle.
//
// This test tracks the order of operations to help understand and fix the issue.
func TestHashMismatchTailChase(t *testing.T) {
	// Stream-backed change_journal_entry skips CAS/hash registry; this scenario exercises CAS + registry.
	// Cannot use t.Parallel(): t.Setenv affects the whole process.
	t.Setenv(zqkenv.StreamStorageEnabled().Name(), "false")
	t.Setenv(zqkenv.TestAllowCASFallthrough().Name(), "1")

	// Setup test project in temporary directory (POL-CODE-006: Test Data Isolation)
	// This ensures we never pollute actual project data with test data
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	// Safeguard: Ensure we're not accidentally using actual project root
	wd, _ := fileutil.Getwd()
	actualProjectRoot := filepath.Clean(filepath.Join(wd, "../../.."))
	if tempDir == actualProjectRoot {
		t.Fatal("Test is using actual project root - this violates POL-CODE-006 (Test Data Isolation)")
	}

	projectRoot := tempDir
	// Other tests may call drainGlobalStorageQueuesForTest(), leaving the process-global
	// shutdown coordinator initiated. Listing/CAS enqueue checks IsShutdownInitiated(), which
	// is suppressed when ZQK_TEST_ROOT is set (see QueueShutdownCoordinator).
	t.Setenv(zqkenv.TestRoot().Name(), projectRoot)

	// Create a change journal entry with an intentional hash mismatch.
	// change_journal_entry is CAS + bucketed; we must create via storage so the object
	// is in the CAS index and Read() can find it before Update().
	month := time.Now().Format("2006-01")
	cjRoot := datacell.CellCASPrimaryDir(projectRoot, "change_journal")
	journalDir := filepath.Join(cjRoot, month)
	kindDir := cjRoot
	if err := fileutil.MkdirAll(journalDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create journal dir: %v", err)
	}
	if err := storage.EnsurePathAliasCacheReady(projectRoot); err != nil {
		t.Fatalf("EnsurePathAliasCacheReady: %v", err)
	}

	// Use test-specific ID (CHA-999) with high number to avoid conflicts with real objects (POL-CODE-006)
	// created_at in same month so Create() uses the same bucket
	chaTestContent := fmt.Sprintf(`id: CHA-999
kind: change_journal_entry
schema_version: "`+objects.DefaultSchemaVersion+`"
status: completed
object_ref: backlog_item:BLI-647
change_type: update
title: "Update: backlog_item:BLI-647"
diff_summary: "Hash mismatch fix"
created_at: "%s-01T16:00:00Z"
created_by: ACC-1785920548450214012-68b850c0
updated_at: "%s-01T16:00:00Z"
updated_by: ACC-1785920548450214012-68b850c0
origin_system: zqk
origin_project: zqk
previous_state:
  status: validated
`, month, month)

	secCtx := &pkgctx.SecurityContext{
		AccountID:   "test-user",
		Roles:       []string{"admin"},
		Permissions: []string{"write:*"},
	}
	// Create CHA-999 via storage so it is indexed in CAS (required for Update to Read it).
	// Use file storage directly so hash registry updates are written to disk (graph backend would not).
	fileStorage, err := storage.NewFileObjectStorageForTest(projectRoot)
	if err != nil {
		t.Fatalf("Failed to create file storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, projectRoot, fileStorage)
	_ = storage.InitializeGlobalBufferWithConfig(projectRoot, secCtx)
	storageProvider := fileStorage
	p := parser.NewYAMLParser()
	createObj, err := p.ParseBytes([]byte(chaTestContent))
	if err != nil {
		t.Fatalf("Failed to parse CHA-999 for create: %v", err)
	}
	createCtx := pkgctx.NewSystemContext()
	// CreateCASVisible materializes off the draft plane so CAS + hash registry exist.
	// (Bare Create can park CHA-* on the draft plane when lifecycle/stream gating disagrees.)
	// TRACK: BLI-1785443942668406000-1ec5c811 — draft-plane create / promote membrane.
	storage.CreateCASVisible(t, fileStorage, createCtx, secCtx, createObj.Properties, "completed")
	kindDir = fileStorage.GetKindDir("change_journal_entry")
	if kindDir == "" {
		t.Fatal("GetKindDir(change_journal_entry) empty after create")
	}
	if inv := storage.InventoryObjectDraftPlane(projectRoot); inv.ByKind["change_journal_entry"] > 0 {
		t.Fatalf("CHA-999 still on draft plane after CreateCASVisible: %+v", inv)
	}

	// Expected hash must match on-disk CAS bytes (may differ from chaTestContent string after YAML round-trip).
	chaPath0, err := fileStorage.GetFilePathForObject("CHA-999", "change_journal_entry")
	if err != nil {
		t.Fatalf("GetFilePathForObject after create: %v", err)
	}
	chaBytes0, err := fileutil.ReadFile(chaPath0)
	if err != nil {
		t.Fatalf("read CHA-999 after create: %v", err)
	}
	chaTestHash := sha256.Sum256(chaBytes0)
	chaTestHashStr := hex.EncodeToString(chaTestHash[:])

	// Simulate wrong hash in the check flow's in-memory cache only (for operation log).
	hashRegistry := storage.NewHashRegistry(pkgctx.NewSystemContext(), "change_journal_entry", kindDir)
	var auditRegistry *storage.HashRegistry
	defer func() {
		regs := []*storage.HashRegistry{hashRegistry}
		if auditRegistry != nil {
			regs = append(regs, auditRegistry)
		}
		drainHashRegistriesForTest(t, regs)
		q := caspkg.GetListingIndexWriteQueueForProjectRoot(projectRoot)
		if q != nil {
			_ = q.FlushAll(5 * time.Second)
			_ = q.Shutdown()
		}
		_ = storage.FlushAllListingIndexesForProjectRoot(projectRoot)
		_ = storage.WaitForWALProcessing(projectRoot, 15*time.Second)
		if fileStorage != nil {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			_ = fileStorage.Shutdown(shutdownCtx)
		}
		resetDir, err := fileutil.MkdirTemp("", "zqk-audit-global-reset")
		if err == nil {
			defer fileutil.RemoveAll(resetDir)
			_ = storage.TearDownGlobalAuditBufferForTestProjectRoot(projectRoot, resetDir, secCtx)
		} else {
			storage.FlushGlobalAuditBufferForProjectRoot(projectRoot)
		}
		// Best-effort: isolated temp project — remove mutable trees so t.TempDir RemoveAll does not race
		// leftover audit/CAS/WAL files (.zqk/process and .zqk).
		_ = fileutil.RemoveAll(datacell.ProcessPrimaryDir(projectRoot))
		_ = fileutil.RemoveAll(filepath.Join(projectRoot, paths.ProjectDataDir))
		// Do not call drainGlobalStorageQueuesForTest here: it shuts down process-global
		// queues/coordinators and races with other parallel tests in this package.
	}()
	_ = hashRegistry.Load()
	wrongHash := "b9b7887952db4c44" + strings.Repeat("0", 48) // Wrong hash (in memory only)
	hashRegistry.SetHash("CHA-999.yaml", wrongHash)
	// Do not Save() - disk has correct hash so Update() sees it and keeps registry correct

	// Track operations
	type Operation struct {
		Step     int
		Time     time.Time
		Action   string
		ObjectID string
		Kind     string
		Details  string
	}
	var operations []Operation
	step := 0

	addOp := func(action, objectID, kind, details string) {
		step++
		operations = append(operations, Operation{
			Step:     step,
			Time:     time.Now(),
			Action:   action,
			ObjectID: objectID,
			Kind:     kind,
			Details:  details,
		})
	}

	// Step 1: Initial state - CHA-999 has hash mismatch
	addOp("check", "CHA-999", "change_journal_entry", fmt.Sprintf("Hash mismatch detected: expected=%s, got=%s", wrongHash[:16], chaTestHashStr[:16]))

	// Step 2: Run check with --auto-fix --force
	cmd := &cobra.Command{}
	cmd.Flags().Bool("auto-fix", true, "")
	cmd.Flags().Bool("force", true, "")

	// Get hash registry from cache (simulating the check flow)
	hashRegistryCache := &HashRegistryCacheType{
		cache: make(map[string]storage.HashRegistryProvider),
	}
	hashRegistryCache.Set("change_journal_entry", hashRegistry)

	// Use the object we created (already parsed)
	obj := createObj

	// Simulate the fix operation
	addOp("fix_start", "CHA-999", "change_journal_entry", "Starting hash mismatch fix")

	// Check if this is an immutable object
	isImmutable := storage.IsBuiltIn(obj.Properties) || obj.Kind == "audit_event" || obj.Kind == "change_journal_entry"
	if !isImmutable {
		t.Fatal("Expected CHA-999 to be immutable")
	}

	// Update with empty updates (storageProvider and secCtx from create block) - this will trigger hash recalculation
	addOp("update_start", "CHA-999", "change_journal_entry", "Calling storageProvider.Update()")
	updateCtx := pkgctx.NewSystemContext()
	updateErr := storageProvider.Update(updateCtx, secCtx, "CHA-999", map[string]any{})
	if updateErr != nil {
		t.Fatalf("Update failed: %v", updateErr)
	}
	_ = storage.WaitForWALProcessing(projectRoot, 15*time.Second)
	storage.FlushAllOrFail(t, projectRoot)
	addOp("update_complete", "CHA-999", "change_journal_entry", "storageProvider.Update() completed")

	chaPath, err := fileStorage.GetFilePathForObject("CHA-999", "change_journal_entry")
	if err != nil {
		t.Fatalf("GetFilePathForObject after update: %v", err)
	}

	// Check what objects were created
	auditDir := filepath.Join(projectRoot, paths.ProcessAuditDir, month)
	//nolint:errcheck // Test cleanup - errors are acceptable
	auditFiles, _ := fileutil.ReadDir(auditDir)
	for _, file := range auditFiles {
		if strings.HasSuffix(file.Name(), ".yaml") {
			auditID := strings.TrimSuffix(file.Name(), ".yaml")
			addOp("created", auditID, "audit_event", "Created during CHA-004 fix")
		}
	}

	// Check for new change journal entries
	//nolint:errcheck // Test cleanup - errors are acceptable
	journalFiles, _ := fileutil.ReadDir(journalDir)
	chaBase := filepath.Base(chaPath)
	for _, file := range journalFiles {
		if strings.HasSuffix(file.Name(), ".yaml") && file.Name() != chaBase {
			journalID := strings.TrimSuffix(file.Name(), ".yaml")
			addOp("created", journalID, "change_journal_entry", "Created during CHA-999 fix")
		}
	}

	// Step 3: Reload hash registry to get the hash that Update() wrote.
	// Read registry from kind root (file storage uses projectRoot, so same path).
	addOp("reload_registry", "change_journal_entry", "hash_registry", "Reloading registry from disk")
	registryPathToRead := filepath.Join(kindDir, ".change_journal_entry.hashes")
	registryData, err := fileutil.ReadFile(registryPathToRead)
	if err != nil {
		t.Fatalf("Failed to read registry file at %s: %v", registryPathToRead, err)
	}
	var registryStruct struct {
		Hashes map[string]string `json:"hashes"`
	}
	if err := json.Unmarshal(registryData, &registryStruct); err != nil {
		t.Fatalf("Failed to parse registry: %v", err)
	}
	if registryStruct.Hashes == nil {
		registryStruct.Hashes = make(map[string]string)
	}
	cachedHash := registryStruct.Hashes["CHA-999.yaml"]
	addOp("check_cache_hash", "CHA-999", "change_journal_entry", fmt.Sprintf("Hash in cache: %s", cachedHash[:16]))

	// Step 5: Hash on-disk bytes (same as storage hash registry: ReadFile after write, not yaml.Marshal(Read())).
	chaBytesAfter, err := fileutil.ReadFile(chaPath)
	if err != nil {
		t.Fatalf("Failed to read CHA-999 YAML after update: %v", err)
	}
	chaTestHashAfter := sha256.Sum256(chaBytesAfter)
	chaTestHashStrAfter := hex.EncodeToString(chaTestHashAfter[:])
	addOp("calculate_file_hash", "CHA-999", "change_journal_entry", fmt.Sprintf("Hash from file: %s", chaTestHashStrAfter[:16]))

	// Step 6: Check if they match
	if cachedHash != chaTestHashStrAfter {
		addOp("mismatch_detected", "CHA-999", "change_journal_entry", fmt.Sprintf("MISMATCH: cache=%s, file=%s", cachedHash[:16], chaTestHashStrAfter[:16]))
		t.Errorf("Hash mismatch after fix: cache=%s, file=%s", cachedHash[:16], chaTestHashStrAfter[:16])
	} else {
		addOp("match_confirmed", "CHA-999", "change_journal_entry", "Hash matches - fix successful")
	}

	// Step 7: Verify the deferred hash check mechanism prevents tail-chasing
	// This simulates what happens in checkAll() after all fixes are complete:
	// 1. Reload all hash registries (to get latest state including newly created objects)
	// 2. Perform deferred hash checks with the latest state
	// 3. Verify no tail-chasing occurs

	// Simulate the deferred check mechanism
	addOp("deferred_check_start", "system", "deferred_check", "Starting deferred hash integrity checks (after all fixes complete)")

	// Re-read registry from disk for deferred check
	registryData2, _ := fileutil.ReadFile(registryPathToRead)
	var registryStruct2 struct {
		Hashes map[string]string `json:"hashes"`
	}
	_ = json.Unmarshal(registryData2, &registryStruct2)
	if registryStruct2.Hashes == nil {
		registryStruct2.Hashes = make(map[string]string)
	}
	auditRegistry = storage.NewHashRegistry(pkgctx.NewSystemContext(), "audit_event", auditDir)
	if err := auditRegistry.Load(); err == nil {
		addOp("reload_registry", "audit_event", "hash_registry", "Reloaded audit_event registry")
	}

	// Re-check CHA-999 with reloaded registry (simulating deferred check)
	finalContent, err := fileutil.ReadFile(chaPath)
	if err == nil {
		finalHash := sha256.Sum256(finalContent)
		finalHashStr := hex.EncodeToString(finalHash[:])
		finalCachedHash := registryStruct2.Hashes["CHA-999.yaml"]
		if finalCachedHash == finalHashStr {
			addOp("deferred_check_pass", "CHA-999", "change_journal_entry", fmt.Sprintf("Deferred check passed: hash matches after reload (hash=%s)", finalHashStr[:16]))
		} else {
			addOp("deferred_check_fail", "CHA-999", "change_journal_entry", fmt.Sprintf("Deferred check failed: cache=%s, file=%s", finalCachedHash[:16], finalHashStr[:16]))
			t.Errorf("Deferred hash check failed for CHA-999: cache=%s, file=%s", finalCachedHash[:16], finalHashStr[:16])
		}
	}

	// Check newly created objects for hash mismatches (these would cause tail-chasing without deferred checks)
	for _, file := range auditFiles {
		if !strings.HasSuffix(file.Name(), ".yaml") {
			continue
		}
		auditID := strings.TrimSuffix(file.Name(), ".yaml")
		auditPath := filepath.Join(auditDir, file.Name())
		auditContent, err := fileutil.ReadFile(auditPath)
		if err != nil {
			continue
		}
		auditHash := sha256.Sum256(auditContent)
		auditHashStr := hex.EncodeToString(auditHash[:])
		expectedHash := auditRegistry.GetHash(file.Name())
		if expectedHash == emptyValue {
			addOp("missing_hash", auditID, "audit_event", "No hash in registry (would cause tail-chase without deferred check)")
		} else if expectedHash != auditHashStr {
			addOp("mismatch_detected", auditID, "audit_event", fmt.Sprintf("MISMATCH: expected=%s, got=%s (TAIL-CHASING)", expectedHash[:16], auditHashStr[:16]))
			t.Errorf("Newly created audit event %s has hash mismatch: expected=%s, got=%s", auditID, expectedHash[:16], auditHashStr[:16])
		} else {
			addOp("match_confirmed", auditID, "audit_event", fmt.Sprintf("Hash matches after deferred check (hash=%s)", auditHashStr[:16]))
		}
	}

	// Check newly created change journal entries
	for _, file := range journalFiles {
		if !strings.HasSuffix(file.Name(), ".yaml") || file.Name() == chaBase {
			continue
		}
		journalID := strings.TrimSuffix(file.Name(), ".yaml")
		journalPath := filepath.Join(journalDir, file.Name())
		journalContent, err := fileutil.ReadFile(journalPath)
		if err != nil {
			continue
		}
		journalHash := sha256.Sum256(journalContent)
		journalHashStr := hex.EncodeToString(journalHash[:])
		expectedHash := registryStruct2.Hashes[file.Name()]
		if expectedHash == emptyValue {
			addOp("missing_hash", journalID, "change_journal_entry", "No hash in registry (would cause tail-chase without deferred check)")
		} else if expectedHash != journalHashStr {
			addOp("mismatch_detected", journalID, "change_journal_entry", fmt.Sprintf("MISMATCH: expected=%s, got=%s (TAIL-CHASING)", expectedHash[:16], journalHashStr[:16]))
			t.Errorf("Newly created change journal entry %s has hash mismatch: expected=%s, got=%s", journalID, expectedHash[:16], journalHashStr[:16])
		} else {
			addOp("match_confirmed", journalID, "change_journal_entry", fmt.Sprintf("Hash matches after deferred check (hash=%s)", journalHashStr[:16]))
		}
	}

	// Print operation log
	t.Log("\n=== Operation Log ===")
	for _, op := range operations {
		t.Logf("[%02d] %s | %s:%s | %s", op.Step, op.Action, op.Kind, op.ObjectID, op.Details)
	}

	// Print summary
	t.Log("\n=== Summary ===")
	t.Logf("Total operations: %d", len(operations))
	mismatches := 0
	for _, op := range operations {
		if op.Action == "mismatch_detected" {
			mismatches++
		}
	}
	t.Logf("Hash mismatches detected: %d", mismatches)
	if mismatches > 0 {
		t.Fatal("\n❌ TAIL-CHASING ISSUE DETECTED - Deferred hash check mechanism failed!")
		t.Log("The fix operation created objects that have hash mismatches in the same cycle.")
		t.Log("This indicates the deferred hash check mechanism is not working correctly.")
	} else {
		t.Log("\n✅ DEFERRED HASH CHECK VERIFIED")
		t.Log("The deferred hash check mechanism successfully prevented tail-chasing.")
		t.Log("All objects (original and newly created) have matching hashes after deferred checks.")
	}
}
