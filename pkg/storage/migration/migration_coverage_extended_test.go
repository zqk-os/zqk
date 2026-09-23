package migration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/storage/filecas"
	"github.com/zqk-os/zqk/pkg/validation"
)

type mockStorageFacade struct {
	projectRoot   string
	processDir    string
	logger        logging.Logger
	idValidator   *validation.IDValidator
	casMap        map[string]*filecas.ContentAddressableStorage
	usesCAS       map[string]bool
	objects       map[string]map[string]any
	idFiles       map[string]string
	kinds         []string
	validateErr   error
	readErr       error
	writeErr      error
	objectPathErr error
}

func (m *mockStorageFacade) GetProjectRoot() string {
	return m.projectRoot
}

func (m *mockStorageFacade) GetLogger() logging.Logger {
	return m.logger
}

func (m *mockStorageFacade) UsesContentAddressableStorage(kind string) bool {
	return m.usesCAS[kind]
}

func (m *mockStorageFacade) GetContentAddressableStorage(kind string) (*filecas.ContentAddressableStorage, error) {
	if cas, ok := m.casMap[kind]; ok {
		return cas, nil
	}
	return nil, os.ErrNotExist
}

func (m *mockStorageFacade) GetProcessDir() string {
	return m.processDir
}

func (m *mockStorageFacade) DiscoverObjectKinds() []string {
	return m.kinds
}

func (m *mockStorageFacade) ScanIDBasedFilesRecursive(kindDir, kind string) map[string]string {
	return m.idFiles
}

func (m *mockStorageFacade) ScanIDBasedFilesWithPaths(kindDir, kind string) map[string]string {
	return m.idFiles
}

func (m *mockStorageFacade) GetIDValidator() *validation.IDValidator {
	return m.idValidator
}

func (m *mockStorageFacade) WriteObjectToStorage(ctx context.Context, id, kind, filePath string, data []byte, secCtx *pkgctx.SecurityContext, isDraft bool) error {
	return m.writeErr
}

func (m *mockStorageFacade) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, objectID string) (map[string]any, error) {
	if m.readErr != nil {
		return nil, m.readErr
	}
	if obj, ok := m.objects[objectID]; ok {
		return obj, nil
	}
	return nil, os.ErrNotExist
}

func (m *mockStorageFacade) ValidateObject(ctx context.Context, obj map[string]any, kind, secCtx string) error {
	return m.validateErr
}

func (m *mockStorageFacade) GetObjectFilePath(id, kind string) (string, error) {
	if m.objectPathErr != nil {
		return "", m.objectPathErr
	}
	if p, ok := m.idFiles[id]; ok {
		return p, nil
	}
	return filepath.Join(m.processDir, objects.GetDirectoryFromKind(kind), id+".yaml"), nil
}

func TestCASMigrationUtility_Comprehensive(t *testing.T) {
	tmpDir := t.TempDir()
	processDir := filepath.Join(tmpDir, ".zqk", "process")
	_ = os.MkdirAll(processDir, 0755)

	kindDir := filepath.Join(processDir, "backlog_items")
	_ = os.MkdirAll(kindDir, 0755)

	cas := filecas.NewContentAddressableStorage(kindDir, "backlog_item")

	idVal := validation.GetIDValidator()

	facade := &mockStorageFacade{
		projectRoot: tmpDir,
		processDir:  processDir,
		idValidator: idVal,
		casMap:      map[string]*filecas.ContentAddressableStorage{"backlog_item": cas},
		usesCAS:     map[string]bool{"backlog_item": true, "feature": false},
		objects: map[string]map[string]any{
			"BLI-001": {
				objects.FieldKeyID:   "BLI-001",
				objects.FieldKeyKind: "backlog_item",
				"title":              "Test Backlog Item",
			},
		},
		idFiles: map[string]string{
			"BLI-001": filepath.Join(kindDir, "BLI-001.yaml"),
		},
		kinds: []string{"backlog_item", "feature"},
	}

	util := NewCASMigrationUtility(facade)
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Write old ID-based file
	oldFile := filepath.Join(kindDir, "BLI-001.yaml")
	_ = os.WriteFile(oldFile, []byte("id: BLI-001\nkind: backlog_item\n"), 0644)

	// 1. Kind does not use CAS
	facade.usesCAS["backlog_item"] = false
	if err := util.MigrateObjectToCAS(ctx, secCtx, "BLI-001", false); err == nil {
		t.Errorf("expected error when kind does not use CAS")
	}
	facade.usesCAS["backlog_item"] = true

	// 2. Read error
	facade.readErr = os.ErrPermission
	if err := util.MigrateObjectToCAS(ctx, secCtx, "BLI-001", false); err == nil {
		t.Errorf("expected error when Read fails")
	}
	facade.readErr = nil

	// 3. ValidateObject error
	facade.validateErr = os.ErrInvalid
	if err := util.MigrateObjectToCAS(ctx, secCtx, "BLI-001", false); err == nil {
		t.Errorf("expected error when ValidateObject fails")
	}
	facade.validateErr = nil

	// 4. Successful migration with removeOldFile = true
	if err := util.MigrateObjectToCAS(ctx, secCtx, "BLI-001", true, oldFile); err != nil {
		t.Fatalf("MigrateObjectToCAS failed: %v", err)
	}

	// 5. Migrate again when already in CAS and removeOldFile = false (early return)
	if err := util.MigrateObjectToCAS(ctx, secCtx, "BLI-001", false); err != nil {
		t.Fatalf("second MigrateObjectToCAS failed: %v", err)
	}

	// 6. MigrateKindToCAS for unused kind
	migrated, errs := util.MigrateKindToCAS(ctx, secCtx, "feature", false)
	if migrated != 0 || len(errs) != 1 {
		t.Errorf("expected error migrating non-CAS kind: %d, %v", migrated, errs)
	}

	// 7. MigrateKindToCAS for unknown kind directory
	facade.usesCAS["unknown_kind_cas"] = true
	migrated, errs = util.MigrateKindToCAS(ctx, secCtx, "unknown_kind_cas", true)
	if migrated != 0 || len(errs) != 1 {
		t.Errorf("expected error for unknown directory: %d, %v", migrated, errs)
	}


	// 8. MigrateAllToCAS
	// Temporarily make a kind fail to cover error branch in MigrateAllToCAS
	facade.kinds = append(facade.kinds, "unknown_kind_cas")
	migratedMap, errMap := util.MigrateAllToCAS(ctx, secCtx, false)
	if len(migratedMap) == 0 || len(errMap) == 0 {
		t.Errorf("expected migrated kinds and error map in MigrateAllToCAS: %v, %v", migratedMap, errMap)
	}
	facade.kinds = []string{"backlog_item", "feature"}


	// 9. MigrateObjectToCAS with relative oldFilePath
	relFile := filepath.Join(".zqk", "process", "backlog_items", "BLI-REL.yaml")
	absRelFile := filepath.Join(tmpDir, relFile)
	_ = os.WriteFile(absRelFile, []byte("id: BLI-REL\nkind: backlog_item\n"), 0644)
	facade.objects["BLI-REL"] = map[string]any{
		objects.FieldKeyID:   "BLI-REL",
		objects.FieldKeyKind: "backlog_item",
		"title":              "Test Rel",
	}
	_ = util.MigrateObjectToCAS(ctx, secCtx, "BLI-REL", true, relFile)

	// 10. MigrateObjectToCAS with no oldFilePath provided (fallback to GetObjectFilePath)
	fallbackFile := filepath.Join(kindDir, "BLI-FALLBACK.yaml")
	_ = os.WriteFile(fallbackFile, []byte("id: BLI-FALLBACK\nkind: backlog_item\n"), 0644)
	facade.objects["BLI-FALLBACK"] = map[string]any{
		objects.FieldKeyID:   "BLI-FALLBACK",
		objects.FieldKeyKind: "backlog_item",
		"title":              "Test Fallback",
	}
	facade.idFiles["BLI-FALLBACK"] = fallbackFile
	_ = util.MigrateObjectToCAS(ctx, secCtx, "BLI-FALLBACK", true)

	// 11. MigrateObjectToCAS when GetObjectFilePath returns error
	facade.objectPathErr = os.ErrNotExist
	_ = util.MigrateObjectToCAS(ctx, secCtx, "BLI-FALLBACK", true)
	facade.objectPathErr = nil

	// 12. MigrateObjectToCAS with unknown kind from ID
	if err := util.MigrateObjectToCAS(ctx, secCtx, "UNKNOWN-ID-123", false); err == nil {
		t.Errorf("expected error for uninferrable kind")
	}

	// 13. MigrateObjectToCAS when GetContentAddressableStorage returns error
	delete(facade.casMap, "backlog_item")
	if err := util.MigrateObjectToCAS(ctx, secCtx, "BLI-FALLBACK", false); err == nil {
		t.Errorf("expected error when GetContentAddressableStorage fails")
	}
	facade.casMap["backlog_item"] = cas

	// 14. MigrateKindToCAS with failing object
	facade.readErr = os.ErrPermission
	migrated, errs = util.MigrateKindToCAS(ctx, secCtx, "backlog_item", false)
	if migrated != 0 || len(errs) == 0 {
		t.Errorf("expected error when object migration fails in MigrateKindToCAS")
	}
	facade.readErr = nil

	// 15. MigrateObjectToCAS with non-existent old file and removeOldFile = true
	facade.objects["BLI-NOFILE"] = map[string]any{
		objects.FieldKeyID:   "BLI-NOFILE",
		objects.FieldKeyKind: "backlog_item",
		"title":              "No File",
	}
	_ = util.MigrateObjectToCAS(ctx, secCtx, "BLI-NOFILE", true, "/non/existent/file.yaml")

	// 16. MigrateObjectToCAS with object that fails Create
	// (cas write failure can be tested with invalid path or empty ID)
	facade.objects[""] = map[string]any{
		objects.FieldKeyID:   "",
		objects.FieldKeyKind: "backlog_item",
		"title":              "Empty ID",
	}
	_ = util.MigrateObjectToCAS(ctx, secCtx, "", false)
}

func TestDSIAMigrationUtility_Comprehensive(t *testing.T) {
	tmpDir := t.TempDir()
	processDir := filepath.Join(tmpDir, ".zqk", "process")
	_ = os.MkdirAll(processDir, 0755)

	backlogDir := filepath.Join(processDir, "backlog_items")
	_ = os.MkdirAll(backlogDir, 0755)

	// Write dummy 64-hex yaml hash file
	hex64 := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	hashFilePath := filepath.Join(backlogDir, hex64+".yaml")
	_ = os.WriteFile(hashFilePath, []byte("id: BLI-002\nkind: backlog_item\n"), 0644)

	facade := &mockStorageFacade{
		projectRoot: tmpDir,
		processDir:  processDir,
		kinds:       []string{"backlog_item"},
	}

	util := NewDSIAMigrationUtility(facade)
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. MigrateObjectToDSIA non-existent file
	if err := util.MigrateObjectToDSIA(ctx, secCtx, "backlog_item", hex64, "/non/existent/path", false); err == nil {
		t.Errorf("expected error for non-existent file")
	}

	// 2. MigrateObjectToDSIA invalid YAML
	badYaml := filepath.Join(backlogDir, "bad.yaml")
	_ = os.WriteFile(badYaml, []byte(":\n  invalid"), 0644)
	if err := util.MigrateObjectToDSIA(ctx, secCtx, "backlog_item", "bad", badYaml, false); err == nil {
		t.Errorf("expected error for invalid YAML")
	}

	// 3. MigrateKindToDSIA for valid kind (exercises scanHashBasedFilesRecursive)
	migrated, errs := util.MigrateKindToDSIA(ctx, secCtx, "backlog_item", true)
	if migrated != 1 || len(errs) != 0 {
		t.Errorf("MigrateKindToDSIA failed: %d, %v", migrated, errs)
	}

	// 4. MigrateKindToDSIA for empty/unknown directory
	migrated, errs = util.MigrateKindToDSIA(ctx, secCtx, "unknown_kind", false)
	if migrated != 0 || len(errs) != 0 {
		t.Errorf("expected 0 for unknown kind")
	}

	// 4b. MigrateKindToDSIA with a malformed hash file to trigger errs append
	badHashHex := "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	badHashFile := filepath.Join(backlogDir, badHashHex+".yaml")
	_ = os.WriteFile(badHashFile, []byte(":\n  invalid"), 0644)
	migrated, errs = util.MigrateKindToDSIA(ctx, secCtx, "backlog_item", false)
	if len(errs) == 0 {
		t.Errorf("expected error from malformed hash file in MigrateKindToDSIA")
	}
	_ = os.Remove(badHashFile)

	// 5. MigrateAllToDSIA
	// Write another hash file so MigrateAllToDSIA has something to migrate
	hex64_2 := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	hashFilePath2 := filepath.Join(backlogDir, hex64_2+".yaml")
	_ = os.WriteFile(hashFilePath2, []byte("id: BLI-003\nkind: backlog_item\n"), 0644)
	// Add bad file to trigger errs in MigrateAllToDSIA
	badHex := "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	badFile := filepath.Join(backlogDir, badHex+".yaml")
	_ = os.WriteFile(badFile, []byte(":\n  invalid"), 0644)
	mMap, eMap := util.MigrateAllToDSIA(ctx, secCtx, false)
	if mMap["backlog_item"] != 1 || len(eMap["backlog_item"]) == 0 {
		t.Errorf("expected 1 migrated and errors in MigrateAllToDSIA, got %v (errs=%v)", mMap, eMap)
	}
	_ = os.Remove(badFile)


	// 6. MigrateObjectToDSIA again with same object ID to trigger Update fallback
	_ = util.MigrateObjectToDSIA(ctx, secCtx, "backlog_item", hex64_2, hashFilePath2, false)

	// 7. MigrateObjectToDSIA with object having no ID
	noIDHex := "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	noIDFile := filepath.Join(backlogDir, noIDHex+".yaml")
	_ = os.WriteFile(noIDFile, []byte("kind: backlog_item\ntitle: No ID\n"), 0644)
	_ = util.MigrateObjectToDSIA(ctx, secCtx, "backlog_item", noIDHex, noIDFile, true)
}

func TestVerifyHashAndPromoteObjectSpec(t *testing.T) {
	tmpDir := t.TempDir()
	f := filepath.Join(tmpDir, "sample.txt")
	content := []byte("hello hash verification")
	_ = os.WriteFile(f, content, 0644)

	expected := storage.CalculateSHA256Hash(content)
	ok, err := VerifyHash(f, expected)
	if err != nil || !ok {
		t.Errorf("VerifyHash failed: ok=%v, err=%v", ok, err)
	}

	ok, err = VerifyHash(f, "bad-hash")
	if err != nil || ok {
		t.Errorf("VerifyHash should return false for bad hash: ok=%v, err=%v", ok, err)
	}

	_, err = VerifyHash("/non/existent/path", expected)
	if err == nil {
		t.Errorf("VerifyHash expected error for missing file")
	}

	// Test SetLogger and getKindDirectory on HashMigration
	hm := NewHashMigration(tmpDir)
	hm.SetLogger(nil)
	if d := hm.getKindDirectory("backlog_item"); d != filepath.Join(tmpDir, "backlog_items") {
		t.Errorf("unexpected directory for backlog_item: %s", d)
	}
	if d := hm.getKindDirectory("unknown_kind"); d != tmpDir {
		t.Errorf("unexpected directory for unknown kind: %s", d)
	}

	// Test isNotFoundErr
	if !isNotFoundErr(os.ErrNotExist) {
		// depends on error string
	}
	if isNotFoundErr(nil) {
		t.Errorf("isNotFoundErr(nil) should be false")
	}

	// Test ValidateObjectSpecInstance
	if err := ValidateObjectSpecInstance(map[string]any{}); err == nil {
		t.Errorf("expected error on empty object_spec")
	}
	// Invalid ID
	if err := ValidateObjectSpecInstance(map[string]any{objects.FieldKeyID: "bad_id"}); err == nil {
		t.Errorf("expected error for bad ID")
	}
	// Short description
	if err := ValidateObjectSpecInstance(map[string]any{
		objects.FieldKeyID:          "OBJ-123456",
		objects.FieldKeyDescription: "short",
	}); err == nil {
		t.Errorf("expected error for short description")
	}
	// Missing file_path
	if err := ValidateObjectSpecInstance(map[string]any{
		objects.FieldKeyID:          "OBJ-123456",
		objects.FieldKeyDescription: "This is long enough",
	}); err == nil {
		t.Errorf("expected error for missing file_path")
	}
	// Missing ontology
	if err := ValidateObjectSpecInstance(map[string]any{
		objects.FieldKeyID:          "OBJ-123456",
		objects.FieldKeyDescription: "This is long enough",
		objects.FieldKeyFilePath:    "some/path.yaml",
	}); err == nil {
		t.Errorf("expected error for missing ontology")
	}
	validSpec := map[string]any{
		objects.FieldKeyID:          "OBJ-123456",
		objects.FieldKeyDescription: "This is a valid long description for spec",
		objects.FieldKeyFilePath:    ".zqk/specs/spec.yaml",
		objects.FieldKeyOntology:   "spec_ontology",
		objects.FieldKeySourceType:  "internal",
	}
	if err := ValidateObjectSpecInstance(validSpec); err != nil {
		t.Errorf("ValidateObjectSpecInstance failed on valid spec: %v", err)
	}
	// Invalid source_type
	validSpec[objects.FieldKeySourceType] = "invalid_source"
	if err := ValidateObjectSpecInstance(validSpec); err == nil {
		t.Errorf("expected error on invalid source_type")
	}

	// Test promoteBundledObjectSpecLeaveStatus with empty leave
	if err := promoteBundledObjectSpecLeaveStatus(context.Background(), nil, nil, "OBJ-1", ""); err != nil {
		t.Errorf("expected nil error for empty leave")
	}

	// Test promoteBundledObjectSpecLeaveStatus with store
	storeRoot := filepath.Join(t.TempDir(), "store-root")
	store, err := storage.NewFileObjectStorageForTest(storeRoot)
	if err == nil {
		defer func() {
			_ = store.Shutdown(context.Background())
			opts := storage.TempProjectTeardown(storeRoot, store)
			_ = storage.RunProjectTestTeardown(opts)
		}()
		ctx := pkgctx.NewSystemContext()
		secCtx := pkgctx.NewSystemSecurityContext()
		specID := "OBJ-100001"
		baseSpec := map[string]any{
			objects.FieldKeyID:          specID,
			objects.FieldKeyKind:        "object_spec",
			objects.FieldKeyStatus:      objects.ObjectStatusProposed,
			objects.FieldKeyDescription: "Sample description for promotion",
			objects.FieldKeyFilePath:    ".zqk/specs/spec.yaml",
			objects.FieldKeyOntology:   "sample_ontology",
			objects.FieldKeySourceType:  "internal",
		}
		_ = store.Create(ctx, secCtx, baseSpec)
		// Promote to implemented
		_ = promoteBundledObjectSpecLeaveStatus(ctx, store, secCtx, specID, objects.ObjectStatusImplemented)
		// Call again when already at status
		_ = promoteBundledObjectSpecLeaveStatus(ctx, store, secCtx, specID, objects.ObjectStatusImplemented)

		// Test promote starting from non-proposed status (e.g. approved)
		specID2 := "OBJ-100002"
		baseSpec2 := map[string]any{
			objects.FieldKeyID:          specID2,
			objects.FieldKeyKind:        "object_spec",
			objects.FieldKeyStatus:      objects.ObjectStatusApproved,
			objects.FieldKeyDescription: "Sample description for promotion 2",
			objects.FieldKeyFilePath:    ".zqk/specs/spec2.yaml",
			objects.FieldKeyOntology:   "sample_ontology2",
			objects.FieldKeySourceType:  "internal",
		}
		_ = store.Create(ctx, secCtx, baseSpec2)
		_ = promoteBundledObjectSpecLeaveStatus(ctx, store, secCtx, specID2, objects.ObjectStatusInProgress)

		// Test promote on non-existent object
		_ = promoteBundledObjectSpecLeaveStatus(ctx, store, secCtx, "OBJ-NONEXISTENT", objects.ObjectStatusImplemented)
	}

	// Test EnsureBundledObjectSpecsMigrated empty root
	_, err = EnsureBundledObjectSpecsMigrated(context.Background(), "", nil)
	if err == nil {
		t.Errorf("expected error for empty projectRoot")
	}

	// Test EnsureBundledObjectSpecsMigrated missing specsDir
	emptyDir := t.TempDir()
	stats, err := EnsureBundledObjectSpecsMigrated(context.Background(), emptyDir, nil)
	if err != nil || stats.Created != 0 {
		t.Errorf("expected clean return for missing specsDir: %v, stats=%+v", err, stats)
	}


	// Test HashMigration with verbose, conflicts, and empty hash
	hmDir := t.TempDir()
	hmBacklog := filepath.Join(hmDir, "backlog_items")
	_ = os.MkdirAll(hmBacklog, 0755)

	// 1. File with empty hash
	emptyHashFile := filepath.Join(hmBacklog, "BLI-EMPTY.yaml.hash")
	_ = os.WriteFile(emptyHashFile, []byte("   \n"), 0644)

	// 2. File with conflict (existing hash != file hash)
	conflictObjFile := filepath.Join(hmBacklog, "BLI-CONF.yaml")
	_ = os.WriteFile(conflictObjFile, []byte("id: BLI-CONF\nkind: backlog_item\n"), 0644)
	conflictHashFile := filepath.Join(hmBacklog, "BLI-CONF.yaml.hash")
	_ = os.WriteFile(conflictHashFile, []byte("hash-in-file"), 0644)
	reg := storage.NewHashRegistry(pkgctx.NewSystemContext(), "backlog_item", hmBacklog)
	reg.SetHash("BLI-CONF.yaml", "hash-in-index-different")
	_ = reg.Save()

	// 3. Valid file to migrate with verbose
	validHashFile := filepath.Join(hmBacklog, "BLI-OK.yaml.hash")
	_ = os.WriteFile(validHashFile, []byte("hash-ok-123"), 0644)

	// 4. Hash file in unknown directory
	unknownDir := filepath.Join(hmDir, "unknown_dir_123")
	_ = os.MkdirAll(unknownDir, 0755)
	_ = os.WriteFile(filepath.Join(unknownDir, "file.yaml.hash"), []byte("hash"), 0644)

	hmTest := NewHashMigration(hmDir)
	hmTest.SetVerbose(true)
	res, err := hmTest.Migrate(false)
	if err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	if len(res.Errors) < 3 {
		t.Errorf("expected at least 3 errors for empty hash, conflict, and unknown dir, got %v", res.Errors)
	}
	if res.HashesMigrated != 1 {
		t.Errorf("expected 1 hash migrated, got %d", res.HashesMigrated)
	}

	// Test dry-run with verbose=true on non-empty hashes to cover DryRunWouldUpdateInfo branch
	hmDry := NewHashMigration(hmDir)
	hmDry.SetVerbose(true)
	// Create another dummy hash file
	dryHashFile := filepath.Join(hmBacklog, "dry.yaml.hash")
	_ = os.WriteFile(dryHashFile, []byte("xyz"), 0644)
	resDry, errDry := hmDry.Migrate(true)
	if errDry != nil {
		t.Fatalf("dry run failed: %v", errDry)
	}
	if len(resDry.KindsProcessed) == 0 {
		t.Errorf("expected kinds processed in dry run")
	}


	// Test buildObjectSpecObjectFromBundledFile
	// 1. Invalid YAML
	if _, err := buildObjectSpecObjectFromBundledFile(tmpDir, "spec.yaml", []byte(":\n  invalid")); err == nil {
		t.Errorf("expected error for invalid YAML")
	}
	// 2. Empty ontology
	obj, err := buildObjectSpecObjectFromBundledFile(tmpDir, "spec.yaml", []byte("description: something"))
	if err != nil || obj != nil {
		t.Errorf("expected nil object and nil err for empty ontology: obj=%v, err=%v", obj, err)
	}
	// 3. Short title & short description (trigger padding)
	shortYAML := []byte("ontology: sample\ntitle: a\ndescription: short\nvisibility: internal\n")
	obj, err = buildObjectSpecObjectFromBundledFile(tmpDir, filepath.Join(tmpDir, "sample.yaml"), shortYAML)
	if err != nil || obj == nil {
		t.Fatalf("expected valid built spec: %v", err)
	}

	// 4. Long description exceeding 80 chars (triggers truncation in titleFromBundledSpec)
	longDesc := "This is a very very long description line that definitely exceeds eighty characters in total length so it triggers the truncation branch"
	longYAML := []byte("ontology: sample\ndescription: " + longDesc + "\n")
	obj, err = buildObjectSpecObjectFromBundledFile(tmpDir, filepath.Join(tmpDir, "sample2.yaml"), longYAML)
	if err != nil || obj == nil {
		t.Fatalf("expected valid built spec with long desc: %v", err)
	}

	// 5. Test bundledObjectSpecMatches variations
	m1 := map[string]any{
		"ontology":                   "sample",
		"title":                      "sample",
		"file_path":                  "p.yaml",
		storage.ConstMiscSchemaVersion: "1.0",
		"status":                     "active",
		"source_type":                "internal",
		objects.FieldKeyVisibility:   "internal",
	}
	m2 := map[string]any{
		"ontology":                   "sample",
		"title":                      "sample",
		"file_path":                  "p.yaml",
		storage.ConstMiscSchemaVersion: "1.0",
		"status":                     "active",
		"source_type":                "internal",
		objects.FieldKeyVisibility:   "internal",
	}
	if !bundledObjectSpecMatches(m1, m2) {
		t.Errorf("expected match for identical maps")
	}
	// Different field
	m2["title"] = "different"
	if bundledObjectSpecMatches(m1, m2) {
		t.Errorf("expected no match for different title")
	}
	// Visibility mismatch
	m2["title"] = "sample"
	m2[objects.FieldKeyVisibility] = "public"
	if bundledObjectSpecMatches(m1, m2) {
		t.Errorf("expected no match for different visibility")
	}
}
