package storage

import (
	caspkg "github.com/lanceman/zqk/pkg/storage/cas"
	"github.com/lanceman/zqk/pkg/zqkenv"

	"github.com/lanceman/zqk/pkg/datacell"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/paths"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/validation"
)

func setupKeystoreTest(t *testing.T) (string, *FileObjectStorage, *pkgctx.SecurityContext) {
	t.Helper()
	tmpDir := t.TempDir()

	// Not in callers: ZQK_TEST_ROOT is process-global.
	t.Setenv(zqkenv.TestRoot().Name(), tmpDir)

	// Create required directory structure
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create process dir: %v", err)
	}

	// Create keystore directory
	keystoreDir := filepath.Join(processDir, "keystore")
	if err := fileutil.MkdirAll(keystoreDir, paths.DirPerm700); err != nil {
		t.Fatalf("Failed to create keystore dir: %v", err)
	}

	// Layout + test-settings + copy object_specs from module
	bootstrapTestRootFromProjectRoot(t, tmpDir, moduleRootFromGoEnv(t))

	// Copy keystore_entry spec to test directory so ID validator can find it
	specsDir := filepath.Join(tmpDir, paths.ProcessInternalObjectSpecsDir)
	projectRoot := moduleRootFromGoEnv(t)
	sourceSpec := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir, "kernel", "keystore_entry.yaml")
	if _, err := fileutil.Stat(sourceSpec); err != nil {
		projectRoot = ""
	} else {
		destSpec := filepath.Join(specsDir, "kernel", "keystore_entry.yaml")
		_ = fileutil.MkdirAll(filepath.Dir(destSpec), paths.DirPerm755)
		if data, err := fileutil.ReadFile(sourceSpec); err == nil {
			_ = fileutil.WriteFile(destSpec, data, paths.FilePerm644) //nolint:errcheck // Test setup
		}
	}

	storage, err := NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	defer func() { _ = storage.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := TempProjectTeardown(tmpDir, storage)
		if err := RunProjectTestTeardown(opts); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	// Force reload of ID patterns from test directory
	if projectRoot != emptyValue {
		testValidator := validation.NewIDValidator(specsDir)
		if err := testValidator.LoadPatterns(); err == nil {
			storage.idValidator = testValidator
		}
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	return tmpDir, storage, secCtx
}

func TestKeystoreEntry_SystemAccess(t *testing.T) {
	tmpDir, storage, systemCtx := setupKeystoreTest(t)
	ctx := context.Background()

	// Verify the spec file was copied and reload validator
	specsDir := filepath.Join(tmpDir, paths.ProcessInternalObjectSpecsDir)
	specFile := filepath.Join(specsDir, "kernel", "keystore_entry.yaml")
	if _, err := fileutil.Stat(specFile); err != nil {
		t.Fatalf("Keystore spec file not found at %s: %v", specFile, err)
	}

	// Create a new validator pointing to the test specs directory
	testValidator := validation.NewIDValidator(specsDir)
	if err := testValidator.LoadPatterns(); err != nil {
		t.Fatalf("Failed to load ID patterns from test directory: %v", err)
	}

	// Replace storage's validator with the test one
	storage.idValidator = testValidator

	// Verify the validator knows about keystore_entry
	prefixes := testValidator.GetValidPrefixes("keystore_entry")
	if len(prefixes) == 0 {
		// Debug: list what kinds the validator knows about
		t.Logf("Spec file exists at %s", specFile)
		t.Logf("Specs directory: %s", specsDir)
		// Try to read the spec file to see if it's valid
		if data, err := fileutil.ReadFile(specFile); err == nil {
			t.Logf("Spec file size: %d bytes", len(data))
			firstChars := 200
			if len(data) < firstChars {
				firstChars = len(data)
			}
			t.Logf("First 200 chars: %s", string(data[:firstChars]))
		}
		t.Fatalf("ID validator doesn't know about keystore_entry kind. Loaded prefixes: %v", prefixes)
	}

	// Create a keystore entry as system
	entry := map[string]any{
		objects.FieldKeyID:             "KEY-001",
		objects.FieldKeyKind:           objects.KindKeystoreEntry,
		objects.FieldKeyTitle:          "Test Key 001",
		objects.FieldKeyAccountID:      "ACC-1785920548450214003-23d25bd5",
		objects.FieldKeyKeyType:        "password",
		objects.FieldKeyCredentialHash: "hashed_password_123",
		objects.FieldKeySalt:           "salt_123",
		objects.FieldKeyRevoked:        false,
		objects.FieldKeyStatus:         objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:   validation.DefaultOriginSystem,
	}

	if err := storage.Create(ctx, systemCtx, entry); err != nil {
		t.Fatalf("Failed to create keystore entry: %v", err)
	}

	// System should be able to read all fields including credential_hash and salt
	readEntry, err := storage.Read(ctx, systemCtx, "KEY-001")
	if err != nil {
		t.Fatalf("Failed to read keystore entry: %v", err)
	}

	if readEntry[objects.FieldKeyCredentialHash] != "hashed_password_123" {
		t.Errorf("Expected credential_hash to be visible to system, got: %v", readEntry[objects.FieldKeyCredentialHash])
	}
	if readEntry[objects.FieldKeySalt] != "salt_123" {
		t.Errorf("Expected salt to be visible to system, got: %v", readEntry[objects.FieldKeySalt])
	}
}

func TestKeystoreEntry_AdminAccess(t *testing.T) {
	_, storage, systemCtx := setupKeystoreTest(t)
	ctx := context.Background()

	// Create a keystore entry as system
	entry := map[string]any{
		objects.FieldKeyID:             "KEY-002",
		objects.FieldKeyKind:           objects.KindKeystoreEntry,
		objects.FieldKeyTitle:          "Test Key 002",
		objects.FieldKeyAccountID:      "ACC-1785920548450214003-23d25bd5",
		objects.FieldKeyKeyType:        "password",
		objects.FieldKeyCredentialHash: "hashed_password_456",
		objects.FieldKeySalt:           "salt_456",
		objects.FieldKeyRevoked:        false,
		objects.FieldKeyStatus:         objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:   validation.DefaultOriginSystem,
	}

	if err := storage.Create(ctx, systemCtx, entry); err != nil {
		t.Fatalf("Failed to create keystore entry: %v", err)
	}

	// Admin should be able to read entry but NOT credential_hash or salt
	adminCtx := pkgctx.NewSecurityContext("account:admin", []string{"admin"}, []string{"read:*", "write:*"})
	readEntry, err := storage.Read(ctx, adminCtx, "KEY-002")
	if err != nil {
		t.Fatalf("Failed to read keystore entry: %v", err)
	}

	// Admin should see metadata but not sensitive fields
	if readEntry[objects.FieldKeyID] != "KEY-002" {
		t.Errorf("Expected id to be visible to admin, got: %v", readEntry[objects.FieldKeyID])
	}
	if readEntry[objects.FieldKeyAccountID] != "ACC-1785920548450214003-23d25bd5" {
		t.Errorf("Expected account_id to be visible to admin, got: %v", readEntry[objects.FieldKeyAccountID])
	}
	if readEntry[objects.FieldKeyKeyType] != "password" {
		t.Errorf("Expected key_type to be visible to admin, got: %v", readEntry[objects.FieldKeyKeyType])
	}

	// Admin should NOT see credential_hash or salt
	if _, exists := readEntry[objects.FieldKeyCredentialHash]; exists {
		t.Errorf("Expected credential_hash to be hidden from admin, but it was present")
	}
	if _, exists := readEntry[objects.FieldKeySalt]; exists {
		t.Errorf("Expected salt to be hidden from admin, but it was present")
	}

	// Best-effort: flush any CAS index activity before TempDir cleanup runs
	// to reduce the chance of background workers keeping files open.
	queue := caspkg.GetGlobalListingIndexWriteQueue()
	if err := queue.FlushAll(2 * time.Second); err != nil {
		t.Logf("FlushAll for CAS index write queue failed during cleanup: %v", err)
	}
	// Also wait for orphan cleanup queue to go idle if it was active
	orphanQueue := caspkg.GetGlobalOrphanCleanupQueue()
	_ = waitForConditionWithTimeout(
		pkgctx.NewSystemContext(),
		func() bool { return !orphanQueue.IsWorkerRunning() },
		2*time.Second,
		10*time.Millisecond,
	)
}

func TestKeystoreEntry_UserOwnAccess(t *testing.T) {
	_, storage, systemCtx := setupKeystoreTest(t)
	ctx := context.Background()

	// Create a keystore entry for a user
	entry := map[string]any{
		objects.FieldKeyID:             "KEY-003",
		objects.FieldKeyKind:           objects.KindKeystoreEntry,
		objects.FieldKeyTitle:          "Test Key 003",
		objects.FieldKeyAccountID:      "ACC-1785920548450214003-23d25bd5",
		objects.FieldKeyKeyType:        "password",
		objects.FieldKeyCredentialHash: "hashed_password_789",
		objects.FieldKeySalt:           "salt_789",
		objects.FieldKeyRevoked:        false,
		objects.FieldKeyStatus:         objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:   validation.DefaultOriginSystem,
	}

	if err := storage.Create(ctx, systemCtx, entry); err != nil {
		t.Fatalf("Failed to create keystore entry: %v", err)
	}

	// User should be able to read their own entry but NOT credential_hash or salt
	userCtx := pkgctx.NewSecurityContext("ACC-1785920548450214003-23d25bd5", []string{"developer"}, []string{"read:*"})
	readEntry, err := storage.Read(ctx, userCtx, "KEY-003")
	if err != nil {
		t.Fatalf("Failed to read keystore entry: %v", err)
	}

	// User should see metadata but not sensitive fields
	if readEntry[objects.FieldKeyID] != "KEY-003" {
		t.Errorf("Expected id to be visible to user, got: %v", readEntry[objects.FieldKeyID])
	}
	if readEntry[objects.FieldKeyAccountID] != "ACC-1785920548450214003-23d25bd5" {
		t.Errorf("Expected account_id to be visible to user, got: %v", readEntry[objects.FieldKeyAccountID])
	}

	// User should NOT see credential_hash or salt
	if _, exists := readEntry[objects.FieldKeyCredentialHash]; exists {
		t.Errorf("Expected credential_hash to be hidden from user, but it was present")
	}
	if _, exists := readEntry[objects.FieldKeySalt]; exists {
		t.Errorf("Expected salt to be hidden from user, but it was present")
	}
}

func TestKeystoreEntry_UserOtherAccess(t *testing.T) {
	_, storage, systemCtx := setupKeystoreTest(t)
	ctx := context.Background()

	// Create a keystore entry for one user
	entry := map[string]any{
		objects.FieldKeyID:             "KEY-004",
		objects.FieldKeyKind:           objects.KindKeystoreEntry,
		objects.FieldKeyTitle:          "Test Key 004",
		objects.FieldKeyAccountID:      "ACC-1785920548450214003-23d25bd5",
		objects.FieldKeyKeyType:        "password",
		objects.FieldKeyCredentialHash: "hashed_password_abc",
		objects.FieldKeySalt:           "salt_abc",
		objects.FieldKeyRevoked:        false,
		objects.FieldKeyStatus:         objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:   validation.DefaultOriginSystem,
	}

	if err := storage.Create(ctx, systemCtx, entry); err != nil {
		t.Fatalf("Failed to create keystore entry: %v", err)
	}

	// Another user should NOT be able to read this entry
	otherUserCtx := pkgctx.NewSecurityContext("ACC-1785920548450214017-87f10a62", []string{"viewer"}, []string{"read:*"})
	readEntry, err := storage.Read(ctx, otherUserCtx, "KEY-004")
	if err != nil {
		t.Fatalf("Failed to read keystore entry: %v", err)
	}

	// Should only see minimal fields (id and kind)
	if readEntry[objects.FieldKeyID] != "KEY-004" {
		t.Errorf("Expected id to be present, got: %v", readEntry[objects.FieldKeyID])
	}
	if readEntry[objects.FieldKeyKind] != objects.KindKeystoreEntry {
		t.Errorf("Expected kind to be present, got: %v", readEntry[objects.FieldKeyKind])
	}

	// Should NOT see account_id or any other fields
	if _, exists := readEntry[objects.FieldKeyAccountID]; exists {
		t.Errorf("Expected account_id to be hidden from other user, but it was present")
	}
	if _, exists := readEntry[objects.FieldKeyCredentialHash]; exists {
		t.Errorf("Expected credential_hash to be hidden from other user, but it was present")
	}
}

func TestKeystoreEntry_ListAccessControl(t *testing.T) {
	_, storage, systemCtx := setupKeystoreTest(t)
	ctx := context.Background()
	storageCtx := pkgctx.GetStorageContext()

	// Create multiple keystore entries for different users
	entries := []map[string]any{
		{
			objects.FieldKeyID:             "KEY-005",
			objects.FieldKeyKind:           objects.KindKeystoreEntry,
			objects.FieldKeyTitle:          "Test Key 005",
			objects.FieldKeyAccountID:      "ACC-1785920548450214003-23d25bd5",
			objects.FieldKeyKeyType:        "password",
			objects.FieldKeyCredentialHash: "hash1",
			objects.FieldKeySalt:           "salt1",
			objects.FieldKeyRevoked:        false,
			objects.FieldKeyStatus:         objects.ObjectStatusActive,
			objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
			objects.FieldKeyOriginProject:  validation.DefaultOriginProject,
			objects.FieldKeyOriginSystem:   validation.DefaultOriginSystem,
		},
		{
			objects.FieldKeyID:             "KEY-006",
			objects.FieldKeyKind:           objects.KindKeystoreEntry,
			objects.FieldKeyTitle:          "Test Key 006",
			objects.FieldKeyAccountID:      "ACC-1785920548450214017-87f10a62",
			objects.FieldKeyKeyType:        "password",
			objects.FieldKeyCredentialHash: "hash2",
			objects.FieldKeySalt:           "salt2",
			objects.FieldKeyRevoked:        false,
			objects.FieldKeyStatus:         objects.ObjectStatusActive,
			objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
			objects.FieldKeyOriginProject:  validation.DefaultOriginProject,
			objects.FieldKeyOriginSystem:   validation.DefaultOriginSystem,
		},
	}

	for _, entry := range entries {
		if err := storage.Create(ctx, systemCtx, entry); err != nil {
			t.Fatalf("Failed to create keystore entry %s: %v", entry[objects.FieldKeyID], err)
		}
	}

	// Developer should only see their own entry
	developerCtx := pkgctx.NewSecurityContext("ACC-1785920548450214003-23d25bd5", []string{"developer"}, []string{"read:*"})
	result, err := storage.List(ctx, developerCtx, storageCtx, ListFilter{
		Kind: objects.KindKeystoreEntry,
	})
	if err != nil {
		t.Fatalf("Failed to list keystore entries: %v", err)
	}

	if len(result.Objects) != 1 {
		t.Errorf("Expected developer to see 1 entry, got %d", len(result.Objects))
	}
	if result.Objects[0][objects.FieldKeyID] != "KEY-005" {
		t.Errorf("Expected developer to see KEY-005, got %v", result.Objects[0][objects.FieldKeyID])
	}
	if _, exists := result.Objects[0][objects.FieldKeyCredentialHash]; exists {
		t.Errorf("Expected credential_hash to be hidden from developer")
	}

	// Admin should see all entries but not credential_hash or salt
	adminCtx := pkgctx.NewSecurityContext("account:admin", []string{"admin"}, []string{"read:*", "write:*"})
	result, err = storage.List(ctx, adminCtx, storageCtx, ListFilter{
		Kind: objects.KindKeystoreEntry,
	})
	if err != nil {
		t.Fatalf("Failed to list keystore entries: %v", err)
	}

	if len(result.Objects) != 2 {
		t.Errorf("Expected admin to see 2 entries, got %d", len(result.Objects))
	}
	for _, obj := range result.Objects {
		if _, exists := obj[objects.FieldKeyCredentialHash]; exists {
			t.Errorf("Expected credential_hash to be hidden from admin")
		}
		if _, exists := obj[objects.FieldKeySalt]; exists {
			t.Errorf("Expected salt to be hidden from admin")
		}
	}
}

func TestKeystoreEntry_UpdateOwnership(t *testing.T) {
	_, storage, systemCtx := setupKeystoreTest(t)
	ctx := context.Background()

	// Create a keystore entry
	entry := map[string]any{
		objects.FieldKeyID:             "KEY-007",
		objects.FieldKeyKind:           objects.KindKeystoreEntry,
		objects.FieldKeyTitle:          "Test Key 007",
		objects.FieldKeyAccountID:      "ACC-1785920548450214003-23d25bd5",
		objects.FieldKeyKeyType:        "password",
		objects.FieldKeyCredentialHash: "hash3",
		objects.FieldKeySalt:           "salt3",
		objects.FieldKeyRevoked:        false,
		objects.FieldKeyStatus:         objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:   validation.DefaultOriginSystem,
	}

	if err := storage.Create(ctx, systemCtx, entry); err != nil {
		t.Fatalf("Failed to create keystore entry: %v", err)
	}

	// Owner should be able to update their own entry
	ownerCtx := pkgctx.NewSecurityContext("ACC-1785920548450214003-23d25bd5", []string{"developer"}, []string{"read:*", "write:*"})
	updates := map[string]any{
		objects.FieldKeyDescription: "Updated description",
	}
	if err := storage.Update(ctx, ownerCtx, "KEY-007", updates); err != nil {
		t.Fatalf("Failed to update keystore entry: %v", err)
	}

	// Owner should NOT be able to update credential_hash or salt
	updates = map[string]any{
		objects.FieldKeyCredentialHash: "new_hash",
	}
	if err := storage.Update(ctx, ownerCtx, "KEY-007", updates); err == nil {
		t.Errorf("Expected error when owner tries to update credential_hash, but got none")
	}

	// Other user should NOT be able to update
	otherUserCtx := pkgctx.NewSecurityContext("ACC-1785920548450214017-87f10a62", []string{"viewer"}, []string{"read:*", "write:*"})
	updates = map[string]any{
		objects.FieldKeyDescription: "Malicious update",
	}
	if err := storage.Update(ctx, otherUserCtx, "KEY-007", updates); err == nil {
		t.Errorf("Expected error when other user tries to update entry, but got none")
	}
}

func TestKeystoreEntry_DeleteOwnership(t *testing.T) {
	_, storage, systemCtx := setupKeystoreTest(t)
	ctx := context.Background()

	// Create a keystore entry
	entry := map[string]any{
		objects.FieldKeyID:             "KEY-008",
		objects.FieldKeyKind:           objects.KindKeystoreEntry,
		objects.FieldKeyTitle:          "Test Key 008",
		objects.FieldKeyAccountID:      "ACC-1785920548450214003-23d25bd5",
		objects.FieldKeyKeyType:        "password",
		objects.FieldKeyCredentialHash: "hash4",
		objects.FieldKeySalt:           "salt4",
		objects.FieldKeyRevoked:        false,
		objects.FieldKeyStatus:         objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:   validation.DefaultOriginSystem,
	}

	if err := storage.Create(ctx, systemCtx, entry); err != nil {
		t.Fatalf("Failed to create keystore entry: %v", err)
	}

	// Owner should be able to delete their own entry (CLI + core hard-delete allow).
	ownerCtx := pkgctx.NewSecurityContext("ACC-1785920548450214003-23d25bd5", []string{"developer"}, []string{"read:*", "write:*", "delete:*"})
	cliCtx := WithTestHardDelete(ctx)
	if err := storage.Delete(cliCtx, ownerCtx, "KEY-008", false); err != nil {
		t.Fatalf("Failed to delete keystore entry: %v", err)
	}

	// Verify entry is deleted
	if _, err := storage.Read(ctx, systemCtx, "KEY-008"); err == nil {
		t.Errorf("Expected entry to be deleted, but it still exists")
	}
}

func TestKeystoreEntry_CreateRestrictions(t *testing.T) {
	_, storage, systemCtx := setupKeystoreTest(t)
	ctx := context.Background()

	// Non-system user should NOT be able to set credential_hash or salt
	userCtx := pkgctx.NewSecurityContext("ACC-1785920548450214003-23d25bd5", []string{"developer"}, []string{"read:*", "write:*"})
	entry := map[string]any{
		objects.FieldKeyID:             "KEY-009",
		objects.FieldKeyKind:           objects.KindKeystoreEntry,
		objects.FieldKeyTitle:          "Test Key 009",
		objects.FieldKeyAccountID:      "ACC-1785920548450214003-23d25bd5",
		objects.FieldKeyKeyType:        "password",
		objects.FieldKeyCredentialHash: "hash5",
		objects.FieldKeySalt:           "salt5",
		objects.FieldKeyRevoked:        false,
		objects.FieldKeyStatus:         objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:   validation.DefaultOriginSystem,
	}

	if err := storage.Create(ctx, userCtx, entry); err == nil {
		t.Errorf("Expected error when non-system user tries to set credential_hash, but got none")
	}

	// Remove credential_hash and salt - should work
	delete(entry, "credential_hash")
	delete(entry, "salt")
	if err := storage.Create(ctx, userCtx, entry); err == nil {
		// This might work, but entry won't be usable without credential_hash
		// Clean up if created
		_ = storage.Delete(ctx, systemCtx, "KEY-009", false) //nolint:errcheck // Cleanup attempt, ignore errors
	}
}

func TestKeystoreEntry_FilePermissions(t *testing.T) {
	_, storage, systemCtx := setupKeystoreTest(t)
	ctx := context.Background()

	// Create a keystore entry
	entry := map[string]any{
		objects.FieldKeyID:             "KEY-010",
		objects.FieldKeyKind:           objects.KindKeystoreEntry,
		objects.FieldKeyTitle:          "Test Key 010",
		objects.FieldKeyAccountID:      "ACC-1785920548450214003-23d25bd5",
		objects.FieldKeyKeyType:        "password",
		objects.FieldKeyCredentialHash: "hash6",
		objects.FieldKeySalt:           "salt6",
		objects.FieldKeyRevoked:        false,
		objects.FieldKeyStatus:         objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:   validation.DefaultOriginSystem,
	}

	if err := storage.Create(ctx, systemCtx, entry); err != nil {
		t.Fatalf("Failed to create keystore entry: %v", err)
	}

	// Get actual file path (may be hash-based if CAS is enabled)
	filePath, err := storage.getObjectFilePath("KEY-010", "keystore_entry")
	if err != nil {
		t.Fatalf("Failed to get keystore file path: %v", err)
	}

	fileInfo, err := fileutil.Stat(filePath)
	if err != nil {
		t.Fatalf("Failed to stat keystore file: %v", err)
	}

	// File should have 0600 permissions (owner read/write only)
	fileMode := fileInfo.Mode().Perm()
	expectedMode := fileutil.FileMode(0600)
	if fileMode != expectedMode {
		t.Errorf("Expected file permissions %o, got %o", expectedMode, fileMode)
	}

	// Directory should have 0700 permissions
	keystoreDir := filepath.Dir(filePath)
	dirInfo, err := fileutil.Stat(keystoreDir)
	if err != nil {
		t.Fatalf("Failed to stat keystore directory: %v", err)
	}

	dirMode := dirInfo.Mode().Perm()
	expectedDirMode := fileutil.FileMode(0700)
	if dirMode != expectedDirMode {
		t.Errorf("Expected directory permissions %o, got %o", expectedDirMode, dirMode)
	}
}

func TestKeystoreEntry_ApplyAccessControl(t *testing.T) {
	storage := &FileObjectStorage{}

	// Test system access
	systemCtx := pkgctx.NewSystemSecurityContext()
	obj := map[string]any{
		objects.FieldKeyID:             "KEY-011",
		objects.FieldKeyKind:           objects.KindKeystoreEntry,
		objects.FieldKeyAccountID:      "ACC-1785920548450214003-23d25bd5",
		objects.FieldKeyKeyType:        "password",
		objects.FieldKeyCredentialHash: "hash7",
		objects.FieldKeySalt:           "salt7",
		objects.FieldKeyRevoked:        false,
	}

	filtered := storage.applyKeystoreAccessControl(obj, systemCtx)
	if !reflect.DeepEqual(filtered, obj) {
		t.Errorf("Expected system to see all fields, got: %v", filtered)
	}

	// Test admin access
	adminCtx := pkgctx.NewSecurityContext("account:admin", []string{"admin"}, []string{"read:*"})
	filtered = storage.applyKeystoreAccessControl(obj, adminCtx)
	if _, exists := filtered[objects.FieldKeyCredentialHash]; exists {
		t.Errorf("Expected credential_hash to be hidden from admin")
	}
	if _, exists := filtered[objects.FieldKeySalt]; exists {
		t.Errorf("Expected salt to be hidden from admin")
	}
	if filtered[objects.FieldKeyID] != "KEY-011" {
		t.Errorf("Expected id to be visible to admin")
	}

	// Test user own access
	userCtx := pkgctx.NewSecurityContext("ACC-1785920548450214003-23d25bd5", []string{"developer"}, []string{"read:*"})
	filtered = storage.applyKeystoreAccessControl(obj, userCtx)
	if _, exists := filtered[objects.FieldKeyCredentialHash]; exists {
		t.Errorf("Expected credential_hash to be hidden from user")
	}
	if filtered[objects.FieldKeyAccountID] != "ACC-1785920548450214003-23d25bd5" {
		t.Errorf("Expected account_id to be visible to user")
	}

	// Test other user access
	otherUserCtx := pkgctx.NewSecurityContext("ACC-1785920548450214017-87f10a62", []string{"viewer"}, []string{"read:*"})
	filtered = storage.applyKeystoreAccessControl(obj, otherUserCtx)
	if len(filtered) != 2 {
		t.Errorf("Expected other user to see only id and kind, got %d fields", len(filtered))
	}
	if filtered[objects.FieldKeyID] != "KEY-011" {
		t.Errorf("Expected id to be visible")
	}
	if filtered[objects.FieldKeyKind] != objects.KindKeystoreEntry {
		t.Errorf("Expected kind to be visible")
	}
	if _, exists := filtered[objects.FieldKeyAccountID]; exists {
		t.Errorf("Expected account_id to be hidden from other user")
	}
}
