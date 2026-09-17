package system

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lanceman/zqk/internal/cli"

	"github.com/spf13/cobra"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/migration/parser"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// TestIntegrityCheck_MultipleViolations tests many simultaneous integrity violations (roles, accounts, audit)
// and batch auto-fix, based on HASH_REGISTRY_SYNC_ANALYSIS.md.
//
// Do not use t.Parallel(): ZQK_TEST_ROOT is process-global; parallel tests can overwrite it and
// storage teardown may flush the wrong root, leaving .zqk/state non-empty and breaking t.TempDir cleanup.
func TestIntegrityCheck_MultipleViolations(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	testRoot := proj.Root
	savedDataDir := zqkenv.TestDataDir().Get()
	t.Cleanup(func() {
		if savedDataDir != emptyValue {
			t.Setenv(zqkenv.TestDataDir().Name(), savedDataDir)
		} else {
		}
	})

	if _, err := setupSystemTestEnvironmentRoot(t, testRoot); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}
	fileStorage, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("Failed to create file storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, testRoot, fileStorage)

	ctx := cli.ContextForProjectAndProfile(testRoot, "test")

	// Simulate the incident from HASH_REGISTRY_SYNC_ANALYSIS.md
	// 8 role files
	rolesDir := filepath.Join(testRoot, paths.ProcessRolesDir)
	if err := fileutil.MkdirAll(rolesDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create roles directory: %v", err)
	}

	roleIDs := []string{"ROL-003", "ROL-004", "ROL-005", "ROL-006", "ROL-007", "ROL-008", "ROL-009", "ROL-010"}
	for _, id := range roleIDs {
		objectFile := filepath.Join(rolesDir, id+".yaml")
		objectContent := fmt.Sprintf(`id: %s
kind: role
schema_version: "`+objects.DefaultSchemaVersion+`"
status: active
title: Role %s
created_at: "2026-01-02T00:00:00Z"
created_by: %s
updated_at: "2026-01-02T00:00:00Z"
updated_by: %s
origin_project: %s
origin_system: %s
namespace_id: %s
`, id, id, pkgctx.SystemAccountID, pkgctx.SystemAccountID, validation.DefaultOriginProject, validation.DefaultOriginSystem, paths.KernelNamespaceID)

		if err := fileutil.WriteFile(objectFile, []byte(objectContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
			t.Fatalf("Failed to create role file %s: %v", id, err)
		}
	}

	// 5 account files
	accountsDir := filepath.Join(testRoot, paths.ProcessAccountsDir)
	if err := fileutil.MkdirAll(accountsDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create accounts directory: %v", err)
	}

	accountNames := []string{"coder-agent", "collective-team-alpha", "observer-agent", "senior-developer", "test-agent"}
	for _, name := range accountNames {
		objectFile := filepath.Join(accountsDir, name+".yaml")
		objectContent := fmt.Sprintf(`id: %s
kind: account
schema_version: "`+objects.DefaultSchemaVersion+`"
status: active
username: %s
created_at: "2026-01-02T00:00:00Z"
created_by: %s
updated_at: "2026-01-02T00:00:00Z"
updated_by: %s
origin_project: %s
origin_system: %s
namespace_id: %s
`, name, name, pkgctx.SystemAccountID, pkgctx.SystemAccountID, validation.DefaultOriginProject, validation.DefaultOriginSystem, paths.KernelNamespaceID)

		if err := fileutil.WriteFile(objectFile, []byte(objectContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
			t.Fatalf("Failed to create account file %s: %v", name, err)
		}
	}

	// 5 audit files (bucketed)
	now := time.Now().UTC()
	month := now.Format("2006-01")
	auditDir := filepath.Join(testRoot, paths.ProcessAuditDir, month)
	if err := fileutil.MkdirAll(auditDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create audit directory: %v", err)
	}

	auditIDs := []string{"AUD-010", "AUD-014", "AUD-017", "AUD-020", "AUD-023"}
	for _, id := range auditIDs {
		objectFile := filepath.Join(auditDir, id+".yaml")
		objectContent := fmt.Sprintf(`id: %s
kind: audit_event
schema_version: "`+objects.DefaultSchemaVersion+`"
status: completed
event_type: test
created_at: "%sT00:00:00Z"
created_by: %s
updated_at: "%sT00:00:00Z"
updated_by: %s
origin_project: %s
origin_system: %s
`, id, now.Format("2006-01-02"), pkgctx.SystemAccountID, now.Format("2006-01-02"), pkgctx.SystemAccountID, validation.DefaultOriginProject, validation.DefaultOriginSystem)

		if err := fileutil.WriteFile(objectFile, []byte(objectContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
			t.Fatalf("Failed to create audit file %s: %v", id, err)
		}
	}

	// Verify all violations are detected
	yamlParser := parser.NewYAMLParser()
	totalViolations := 0

	// Check roles
	for _, id := range roleIDs {
		objectFile := filepath.Join(rolesDir, id+".yaml")
		parsedObj, err := yamlParser.ParseFile(objectFile)
		if err != nil {
			continue
		}
		issues, _ := checkIntegrity(ctx, &cobra.Command{}, parsedObj, objectFile, "role")
		for _, issue := range issues {
			if issue.Category == "integrity" {
				totalViolations++
			}
		}
	}

	// Check accounts
	for _, name := range accountNames {
		objectFile := filepath.Join(accountsDir, name+".yaml")
		parsedObj, err := yamlParser.ParseFile(objectFile)
		if err != nil {
			continue
		}
		issues, _ := checkIntegrity(ctx, &cobra.Command{}, parsedObj, objectFile, "account")
		for _, issue := range issues {
			if issue.Category == "integrity" {
				totalViolations++
			}
		}
	}

	// Check audit events
	for _, id := range auditIDs {
		objectFile := filepath.Join(auditDir, id+".yaml")
		parsedObj, err := yamlParser.ParseFile(objectFile)
		if err != nil {
			continue
		}
		issues, _ := checkIntegrity(ctx, &cobra.Command{}, parsedObj, objectFile, "audit_event")
		for _, issue := range issues {
			if issue.Category == "integrity" {
				totalViolations++
			}
		}
	}

	expectedViolations := len(roleIDs) + len(accountNames) + len(auditIDs) // 18 total
	if totalViolations < expectedViolations {
		t.Errorf("Expected at least %d violations, got %d", expectedViolations, totalViolations)
	}

	// Test batch auto-fix
	dummyCmd := &cobra.Command{}
	dummyCmd.Flags().Bool("auto-fix", true, "")
	dummyCmd.Flags().Bool("force", false, "")

	allFixed := true
	for _, id := range roleIDs {
		objectFile := filepath.Join(rolesDir, id+".yaml")
		parsedObj, err := yamlParser.ParseFile(objectFile)
		if err != nil {
			continue
		}
		issues, _ := checkIntegrity(ctx, &cobra.Command{}, parsedObj, objectFile, "role")
		if len(issues) > 0 && issues[0].AutoFixable {
			registry := storage.NewHashRegistry(pkgctx.NewSystemContext(), "role", rolesDir)
			_ = registry.Load() //nolint:errcheck // Test setup - registry may not exist yet
			issuesForFix := []Issue{
				{
					Tier:        3,
					Category:    "integrity",
					Message:     "No integrity hash recorded for this file",
					AutoFixable: true,
				},
			}
			objectIDCache := NewObjectIDCache()
			fixed := autoFixIssues(ctx, dummyCmd, parsedObj, objectFile, "role", issuesForFix, registry, nil, objectIDCache, nil)
			if len(fixed) == 0 {
				allFixed = false
			}
			// Each NewHashRegistry may start a save worker; idle timeout is long. Shut down before
			// t.TempDir cleanup so no goroutine holds files under testRoot.
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			_ = registry.InitiateShutdown()
			_ = registry.Drain(shutdownCtx) //nolint:errcheck // best-effort test cleanup
			cancel()
		}
	}

	if !allFixed {
		t.Error("Expected all violations to be auto-fixable")
	}
}

// TestIntegrityCheck_ModificationPatterns tests different file modification patterns
// Based on real-world scenarios: partial edits, whitespace changes, field additions
// TestIntegrityCheck_MixedViolationTypes: do not run in parallel (ZQK_TEST_DATA_DIR is process-global; avoid 001 path and TempDir cleanup errors).
func TestIntegrityCheck_MixedViolationTypes(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	testRoot := proj.Root
	savedDataDir := zqkenv.TestDataDir().Get()
	t.Cleanup(func() {
		if savedDataDir != emptyValue {
			t.Setenv(zqkenv.TestDataDir().Name(), savedDataDir)
		} else {
		}
	})

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
	// Full storage teardown (CAS flush, WAL wait, audit buffer, scrub) so t.TempDir() can remove .zqk and .zqk/process.
	backlogDir := fileStorage.GetKindDir(objects.KindBacklogItem)
	if err := fileutil.MkdirAll(backlogDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create backlog directory: %v", err)
	}
	projectRootForCheck := fileStorage.GetProjectRoot()

	checkCtx := cli.ContextForProjectAndProfile(projectRootForCheck, "test")

	// Create some objects via CLI (will have hashes)
	cliCreatedIDs := []string{"BLI-017", "BLI-018", "BLI-019"}
	for _, id := range cliCreatedIDs {
		obj := map[string]any{
			objects.FieldKeyID:            id,
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyStatus:        objects.ObjectStatusExploring,
			objects.FieldKeyTitle:         fmt.Sprintf("CLI Created %s", id),
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
	}
	storage.FlushAllOrFail(t, projectRootForCheck)
	for _, id := range cliCreatedIDs {
		ensureHashRegistryEntryForObject(t, fileStorage, "backlog_item", id, backlogDir)
	}

	// Create some files outside CLI (missing hashes)
	directCreatedIDs := []string{"BLI-020", "BLI-021", "BLI-022"}
	for _, id := range directCreatedIDs {
		objectFile := filepath.Join(backlogDir, id+".yaml")
		objectContent := fmt.Sprintf(`id: %s
kind: backlog_item
schema_version: "`+objects.DefaultSchemaVersion+`"
status: exploring
title: Direct Created %s
created_at: "2026-01-02T00:00:00Z"
created_by: %s
updated_at: "2026-01-02T00:00:00Z"
updated_by: %s
origin_project: %s
origin_system: %s
namespace_id: %s
priority_tier: P3
`, id, id, pkgctx.SystemAccountID, pkgctx.SystemAccountID, validation.DefaultOriginProject, validation.DefaultOriginSystem, paths.KernelNamespaceID)

		if err := fileutil.WriteFile(objectFile, []byte(objectContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
			t.Fatalf("Failed to create %s: %v", id, err)
		}
	}

	// Modify some CLI-created files (hash mismatches). Resolve path in case storage uses CAS (hash-named files).
	for _, id := range cliCreatedIDs[:2] { // Modify first 2
		objectFile, err := resolveBacklogItemPathByScan(backlogDir, id)
		if err != nil {
			t.Fatalf("Failed to resolve path for %s: %v", id, err)
		}
		content, err := fileutil.ReadFile(objectFile)
		if err != nil {
			t.Fatalf("Failed to read %s: %v", id, err)
		}
		modifiedContent := string(content) + "\n# Modified outside CLI\n"
		if err := fileutil.WriteFile(objectFile, []byte(modifiedContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
			t.Fatalf("Failed to modify %s: %v", id, err)
		}
	}

	// Use the same check path as production (checkKindObjects -> CAS integrity), then count violations
	results, err := checkKindObjects(checkCtx, &cobra.Command{}, "backlog_item", nil)
	if err != nil {
		t.Fatalf("checkKindObjects failed: %v", err)
	}

	missingHashCount := 0
	mismatchCount := 0
	for _, result := range results {
		for _, issue := range result.Issues {
			if issue.Category != "integrity" {
				continue
			}
			if strings.Contains(issue.Message, "No integrity hash recorded") || strings.Contains(issue.Message, "object not in CAS index") {
				missingHashCount++
			} else if strings.Contains(issue.Message, "Hash mismatch detected") || strings.Contains(issue.Message, "content hash does not match") {
				mismatchCount++
			}
		}
	}

	// Verify counts
	expectedMissingHash := len(directCreatedIDs) // 3 files created outside CLI
	expectedMismatch := 2                        // 2 CLI-created files modified

	if missingHashCount != expectedMissingHash {
		t.Errorf("Expected %d missing hash violations, got %d", expectedMissingHash, missingHashCount)
	}

	if mismatchCount != expectedMismatch {
		t.Errorf("Expected %d hash mismatch violations, got %d", expectedMismatch, mismatchCount)
	}
}

// TestIntegrityCheck_RecoveryOrder tests that recovery handles violations in correct order
// Missing hashes should be auto-fixed first, then hash mismatches require --force
