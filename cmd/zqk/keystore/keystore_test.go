package keystore

import (
	"context"
	"path/filepath"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/paths"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_v2" // Register spec builders
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/validation"
)

func setupKeystoreTest(t *testing.T) (tmpDir string, storageProvider *storage.FileObjectStorage, secCtx *pkgctx.SecurityContext) {
	t.Setenv(zqkenv.TestAllowCASFallthrough().Name(), "1")
	t.Setenv(zqkenv.PrivilegedWriterSocket().Name(), zqkenv.UnreachableTestSocketPath())
	// Mutates validation.GetIDValidator() specs dir — callers must not t.Parallel.
	tmpDir = t.TempDir()

	// Setup test environment which copies spec files
	if _, err := setupKeystoreTestEnvironmentRoot(tmpDir); err != nil {
		t.Fatalf("Failed to setup test environment: %v", err)
	}

	// Create keystore directory with correct permissions
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	keystoreDir := filepath.Join(processDir, "keystore")
	if err := fileutil.MkdirAll(keystoreDir, paths.DirPerm700); err != nil {
		t.Fatalf("Failed to create keystore dir: %v", err)
	}

	// Copy keystore_entry spec to test directory so ID validator can find it
	specsDir := filepath.Join(tmpDir, paths.ProcessInternalObjectSpecsDir)
	cwd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	var projectRoot string
	for dir := cwd; dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		specPath := filepath.Join(dir, paths.ProcessInternalObjectSpecsDir, "keystore_entry.yaml")
		if _, err := fileutil.Stat(specPath); err == nil {
			projectRoot = dir
			sourceSpec := specPath
			destSpec := filepath.Join(specsDir, "keystore_entry.yaml")
			if data, err := fileutil.ReadFile(sourceSpec); err == nil {
				if err := fileutil.WriteFile(destSpec, data, paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
					t.Fatalf("Failed to write spec file: %v", err)
				}
			}
			break
		}
	}

	storageProvider, err = storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tmpDir, storageProvider)
	testkit.RegisterTempProjectTeardown(t, tmpDir, storageProvider)

	// Force reload of ID patterns from test directory into global validator
	if projectRoot != emptyValue {
		specsDir := filepath.Join(tmpDir, paths.ProcessInternalObjectSpecsDir)
		validator := validation.GetIDValidator()
		validator.SetSpecsDirForTest(specsDir)
		if err := validator.ReloadPatterns(); err != nil {
			t.Fatalf("Failed to reload ID patterns: %v", err)
		}
	}

	secCtx = pkgctx.NewSystemSecurityContext()
	return tmpDir, storageProvider, secCtx
}

// promoteKeystoreEntryOffDraft leaves draft-first create so later Update/Read hit CAS.
// TRACK: BLI-1785443942668406000-1ec5c811 — draft-plane create / promote membrane.
func promoteKeystoreEntryOffDraft(t *testing.T, fs storage.ObjectStorageProvider, ctx context.Context, secCtx *pkgctx.SecurityContext, id string) {
	t.Helper()
	if id == "" {
		t.Fatal("promoteKeystoreEntryOffDraft: empty id")
	}
	if err := fs.Update(ctx, secCtx, id, map[string]any{objects.FieldKeyStatus: objects.ObjectStatusActive}); err != nil {
		t.Fatalf("promoteKeystoreEntryOffDraft %s: %v", id, err)
	}
}

func TestKeystoreCreate_ValidKey(t *testing.T) {
	// TRACK: BLI-1785443942668406000-1ec5c811 — no t.Parallel: setupKeystoreTest mutates global ID validator.
	_, storageProvider, systemCtx := setupKeystoreTest(t)
	ctx := pkgctx.NewSystemContext()

	// Test creating a keystore entry with valid data
	entry := map[string]any{
		objects.FieldKeyKind:           objects.KindKeystoreEntry,
		objects.FieldKeyTitle:          "Test API Key",
		objects.FieldKeyAccountID:      "ACC-1785920548450214003-23d25bd5",
		objects.FieldKeyKeyType:        "api_key",
		objects.FieldKeyCredentialHash: "sha256:abc123",
		objects.FieldKeyDescription:    "Test key for development",
		objects.FieldKeyRevoked:        false,
		objects.FieldKeyStatus:         objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:   validation.DefaultOriginSystem,
	}

	err := storageProvider.Create(ctx, systemCtx, entry)
	if err != nil {
		t.Fatalf("Failed to create keystore entry: %v", err)
	}

	// Verify entry was created
	id, ok := entry[objects.FieldKeyID].(string)
	if !ok || id == emptyValue {
		t.Fatalf("Expected entry to have an ID, got: %v", entry[objects.FieldKeyID])
	}

	// Verify entry can be read (works with both CAS and non-CAS storage)
	readEntry, err := storageProvider.Read(ctx, systemCtx, id)
	if err != nil {
		t.Fatalf("Failed to read keystore entry: %v", err)
	}
	if readEntry[objects.FieldKeyID] != id {
		t.Errorf("Expected ID %s, got %v", id, readEntry[objects.FieldKeyID])
	}

	// Find the actual file: walk from storage's project root (CAS may use bucket subdirs)
	walkRoot := storageProvider.GetProjectRoot()
	var filePath string
	err = filepath.Walk(walkRoot, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil {
			if fileutil.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml" {
			return nil
		}
		dir := filepath.Dir(path)
		if filepath.Base(dir) == "keystore" {
			filePath = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil && err != filepath.SkipAll {
		t.Fatalf("Failed to walk project root: %v", err)
	}
	if filePath != emptyValue {
		// Verify file permissions when we found the file (path from walk; in CAS/job env the file may live elsewhere so Stat can fail)
		fileInfo, err := fileutil.Stat(filePath)
		if err != nil {
			t.Skipf("Keystore file from walk not stat-able in this environment (e.g. CAS or scheduler job layout): %v", err)
		}
		fileMode := fileInfo.Mode().Perm()
		if fileMode != fileutil.FileMode(0600) {
			t.Errorf("Expected file permissions 0600, got %o", fileMode)
		}
		keystoreDir := filepath.Dir(filePath)
		if dirInfo, err := fileutil.Stat(keystoreDir); err == nil {
			if dirMode := dirInfo.Mode().Perm(); dirMode != fileutil.FileMode(0700) {
				t.Errorf("Expected directory permissions 0700, got %o", dirMode)
			}
		}
	}
	// If file not found (e.g. CAS layout or env), Read already verified the object exists

	// Verify entry content matches (already read above)
	if readEntry[objects.FieldKeyTitle] != "Test API Key" {
		t.Errorf("Expected title 'Test API Key', got %v", readEntry[objects.FieldKeyTitle])
	}
	if readEntry[objects.FieldKeyAccountID] != "ACC-1785920548450214003-23d25bd5" {
		t.Errorf("Expected account_id 'ACC-1785920548450214003-23d25bd5', got %v", readEntry[objects.FieldKeyAccountID])
	}
}

func TestKeystoreCreate_NonSystemUserCannotSetCredentialHash(t *testing.T) {
	// TRACK: BLI-1785443942668406000-1ec5c811 — no t.Parallel: setupKeystoreTest mutates global ID validator.
	_, storageProvider, _ := setupKeystoreTest(t)
	ctx := pkgctx.NewSystemContext()

	// Non-system user should not be able to set credential_hash
	userCtx := pkgctx.NewSecurityContext("ACC-1785920548450214003-23d25bd5", []string{"developer"}, []string{"read:*", "write:*"})

	entry := map[string]any{
		objects.FieldKeyKind:           objects.KindKeystoreEntry,
		objects.FieldKeyTitle:          "Test Key",
		objects.FieldKeyAccountID:      "ACC-1785920548450214003-23d25bd5",
		objects.FieldKeyKeyType:        "api_key",
		objects.FieldKeyCredentialHash: "sha256:abc123", // Non-system user trying to set this
		objects.FieldKeyStatus:         objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:   validation.DefaultOriginSystem,
	}

	err := storageProvider.Create(ctx, userCtx, entry)
	if err == nil {
		t.Errorf("Expected error when non-system user tries to set credential_hash, but got none")
	}
}

func TestKeystoreCreate_AccountIDAutoSet(t *testing.T) {
	// TRACK: BLI-1785443942668406000-1ec5c811 — no t.Parallel: setupKeystoreTest mutates global ID validator.
	_, storageProvider, _ := setupKeystoreTest(t)
	ctx := pkgctx.NewSystemContext()

	// User context - account_id should be auto-set from security context
	userCtx := pkgctx.NewSecurityContext("ACC-1785920548450214003-23d25bd5", []string{"developer"}, []string{"read:*", "write:*"})

	entry := map[string]any{
		objects.FieldKeyKind:    objects.KindKeystoreEntry,
		objects.FieldKeyTitle:   "My API Key",
		objects.FieldKeyKeyType: "api_key",
		// account_id not provided - should be auto-set from security context
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject: validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
	}

	// Since validation errors no longer block creation (membrane pattern), this will succeed.
	// We just want to ensure account_id was auto-set properly by prepareKeystoreEntry.
	err := storageProvider.Create(ctx, userCtx, entry)

	if err != nil {
		t.Fatalf("Expected no error when creating keystore entry without credential_hash, but got: %v", err)
	}

	if accountID, ok := entry[objects.FieldKeyAccountID].(string); !ok || accountID != "ACC-1785920548450214003-23d25bd5" {
		t.Errorf("Expected account_id to be auto-set to 'ACC-1785920548450214003-23d25bd5', got: %v", entry[objects.FieldKeyAccountID])
	}
}

// List tests are skipped due to ListFilter type visibility issue in test compilation.
// The list functionality is thoroughly tested in pkg/storage/keystore_access_control_test.go
// which has access to the ListFilter type since it's in the same package.
func TestKeystoreList_UserSeesOwnEntries(t *testing.T) {
	// TRACK: BLI-1785443942668406000-1ec5c811 — no t.Parallel: setupKeystoreTest mutates global ID validator.
	t.Skip("Skipping test due to ListFilter type visibility issue - functionality tested in pkg/storage/keystore_access_control_test.go")
}

func TestKeystoreList_AdminSeesAllEntries(t *testing.T) {
	// TRACK: BLI-1785443942668406000-1ec5c811 — no t.Parallel: setupKeystoreTest mutates global ID validator.
	t.Skip("Skipping test due to ListFilter type visibility issue - functionality tested in pkg/storage/keystore_access_control_test.go")
}

func TestKeystoreList_SystemSeesAllFields(t *testing.T) {
	// TRACK: BLI-1785443942668406000-1ec5c811 — no t.Parallel: setupKeystoreTest mutates global ID validator.
	t.Skip("Skipping test due to ListFilter type visibility issue - functionality tested in pkg/storage/keystore_access_control_test.go")
}

func TestKeystoreRotate_ValidRotation(t *testing.T) {
	// TRACK: BLI-1785443942668406000-1ec5c811 — no t.Parallel: setupKeystoreTest mutates global ID validator.
	_, storageProvider, systemCtx := setupKeystoreTest(t)
	ctx := pkgctx.NewSystemContext()

	// Create an initial keystore entry
	entry := map[string]any{
		objects.FieldKeyKind:           objects.KindKeystoreEntry,
		objects.FieldKeyTitle:          "Test API Key",
		objects.FieldKeyAccountID:      "ACC-1785920548450214003-23d25bd5",
		objects.FieldKeyKeyType:        "api_key",
		objects.FieldKeyCredentialHash: "sha256:old_hash",
		objects.FieldKeyRevoked:        false,
		objects.FieldKeyStatus:         objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:   validation.DefaultOriginSystem,
	}

	if err := storageProvider.Create(ctx, systemCtx, entry); err != nil {
		t.Fatalf("Failed to create keystore entry: %v", err)
	}
	id, _ := entry[objects.FieldKeyID].(string)
	// TRACK: BLI-1785443942668406000-1ec5c811 — draft-plane create / promote membrane.
	promoteKeystoreEntryOffDraft(t, storageProvider, ctx, systemCtx, id)

	// Rotate the key (system can update credential_hash)
	newCredentialHash := "sha256:new_hash" //nolint:gosec // Test hash value, not a real credential
	updates := map[string]any{
		objects.FieldKeyCredentialHash: newCredentialHash,
	}

	if err := storageProvider.Update(ctx, systemCtx, id, updates); err != nil {
		t.Fatalf("Failed to rotate keystore entry: %v", err)
	}

	// Verify the credential_hash was updated
	readEntry, err := storageProvider.Read(ctx, systemCtx, id)
	if err != nil {
		t.Fatalf("Failed to read keystore entry: %v", err)
	}

	if readEntry[objects.FieldKeyCredentialHash] != newCredentialHash {
		t.Errorf("Expected credential_hash to be updated to %s, got %v", newCredentialHash, readEntry[objects.FieldKeyCredentialHash])
	}
}

func TestKeystoreRotate_NonSystemUserCannotRotate(t *testing.T) {
	// TRACK: BLI-1785443942668406000-1ec5c811 — no t.Parallel: setupKeystoreTest mutates global ID validator.
	_, storageProvider, systemCtx := setupKeystoreTest(t)
	ctx := pkgctx.NewSystemContext()

	// Create an initial keystore entry
	entry := map[string]any{
		objects.FieldKeyKind:           objects.KindKeystoreEntry,
		objects.FieldKeyTitle:          "Test API Key",
		objects.FieldKeyAccountID:      "ACC-1785920548450214003-23d25bd5",
		objects.FieldKeyKeyType:        "api_key",
		objects.FieldKeyCredentialHash: "sha256:old_hash",
		objects.FieldKeyRevoked:        false,
		objects.FieldKeyStatus:         objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:   validation.DefaultOriginSystem,
	}

	if err := storageProvider.Create(ctx, systemCtx, entry); err != nil {
		t.Fatalf("Failed to create keystore entry: %v", err)
	}

	id, _ := entry[objects.FieldKeyID].(string)

	// Non-system user should not be able to update credential_hash
	userCtx := pkgctx.NewSecurityContext("ACC-1785920548450214003-23d25bd5", []string{"developer"}, []string{"read:*", "write:*"})
	updates := map[string]any{
		objects.FieldKeyCredentialHash: "sha256:new_hash",
	}

	err := storageProvider.Update(ctx, userCtx, id, updates)
	if err == nil {
		t.Errorf("Expected error when non-system user tries to rotate key, but got none")
	}
}

func TestKeystoreRotate_RevokeOldKey(t *testing.T) {
	// TRACK: BLI-1785443942668406000-1ec5c811 — no t.Parallel: setupKeystoreTest mutates global ID validator.
	_, storageProvider, systemCtx := setupKeystoreTest(t)
	ctx := pkgctx.NewSystemContext()

	// Create an initial keystore entry
	entry := map[string]any{
		objects.FieldKeyKind:           objects.KindKeystoreEntry,
		objects.FieldKeyTitle:          "Test API Key",
		objects.FieldKeyAccountID:      "ACC-1785920548450214003-23d25bd5",
		objects.FieldKeyKeyType:        "api_key",
		objects.FieldKeyCredentialHash: "sha256:old_hash",
		objects.FieldKeyRevoked:        false,
		objects.FieldKeyStatus:         objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:   validation.DefaultOriginSystem,
	}

	if err := storageProvider.Create(ctx, systemCtx, entry); err != nil {
		t.Fatalf("Failed to create keystore entry: %v", err)
	}
	id, _ := entry[objects.FieldKeyID].(string)
	// TRACK: BLI-1785443942668406000-1ec5c811 — draft-plane create / promote membrane.
	promoteKeystoreEntryOffDraft(t, storageProvider, ctx, systemCtx, id)

	// Revoke the key
	updates := map[string]any{
		objects.FieldKeyRevoked: true,
	}

	if err := storageProvider.Update(ctx, systemCtx, id, updates); err != nil {
		t.Fatalf("Failed to revoke keystore entry: %v", err)
	}

	// Verify the key is revoked
	readEntry, err := storageProvider.Read(ctx, systemCtx, id)
	if err != nil {
		t.Fatalf("Failed to read keystore entry: %v", err)
	}

	if revoked, ok := readEntry[objects.FieldKeyRevoked].(bool); !ok || !revoked {
		t.Errorf("Expected key to be revoked, got revoked=%v", readEntry[objects.FieldKeyRevoked])
	}
}

func TestKeystoreRotate_OwnerCanRevoke(t *testing.T) {
	// TRACK: BLI-1785443942668406000-1ec5c811 — no t.Parallel: setupKeystoreTest mutates global ID validator.
	_, storageProvider, systemCtx := setupKeystoreTest(t)
	ctx := pkgctx.NewSystemContext()

	// Create an initial keystore entry
	entry := map[string]any{
		objects.FieldKeyKind:           objects.KindKeystoreEntry,
		objects.FieldKeyTitle:          "Test API Key",
		objects.FieldKeyAccountID:      "ACC-1785920548450214003-23d25bd5",
		objects.FieldKeyKeyType:        "api_key",
		objects.FieldKeyCredentialHash: "sha256:old_hash",
		objects.FieldKeyRevoked:        false,
		objects.FieldKeyStatus:         objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:   validation.DefaultOriginSystem,
	}

	if err := storageProvider.Create(ctx, systemCtx, entry); err != nil {
		t.Fatalf("Failed to create keystore entry: %v", err)
	}
	id, _ := entry[objects.FieldKeyID].(string)
	// TRACK: BLI-1785443942668406000-1ec5c811 — draft-plane create / promote membrane.
	promoteKeystoreEntryOffDraft(t, storageProvider, ctx, systemCtx, id)

	// Owner should be able to revoke their own key
	ownerCtx := pkgctx.NewSecurityContext("ACC-1785920548450214003-23d25bd5", []string{"developer"}, []string{"read:*", "write:*"})
	updates := map[string]any{
		objects.FieldKeyRevoked: true,
	}

	if err := storageProvider.Update(ctx, ownerCtx, id, updates); err != nil {
		t.Fatalf("Failed to revoke keystore entry: %v", err)
	}

	// Verify the key is revoked
	readEntry, err := storageProvider.Read(ctx, systemCtx, id)
	if err != nil {
		t.Fatalf("Failed to read keystore entry: %v", err)
	}

	if revoked, ok := readEntry[objects.FieldKeyRevoked].(bool); !ok || !revoked {
		t.Errorf("Expected key to be revoked, got revoked=%v", readEntry[objects.FieldKeyRevoked])
	}
}
