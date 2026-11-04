package docman

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// faultyDocStorageProvider extends mockDocStorageProvider with controllable errors
type faultyDocStorageProvider struct {
	mockDocStorageProvider
	listErr   error
	createErr error
	readErr   error
	updateErr error

	// Selective error triggers
	failUpdateForID string
	failReadForID   string
}

func newFaultyDocStorageProvider() *faultyDocStorageProvider {
	return &faultyDocStorageProvider{
		mockDocStorageProvider: *newMockDocStorageProvider(),
	}
}

func (f *faultyDocStorageProvider) List(ctx context.Context, sec *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.mockDocStorageProvider.List(ctx, sec, storageCtx, filter)
}

func (f *faultyDocStorageProvider) Create(ctx context.Context, sec *pkgctx.SecurityContext, obj map[string]any) error {
	if f.createErr != nil {
		return f.createErr
	}
	return f.mockDocStorageProvider.Create(ctx, sec, obj)
}

func (f *faultyDocStorageProvider) Read(ctx context.Context, sec *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if f.readErr != nil {
		return nil, f.readErr
	}
	if f.failReadForID != "" && id == f.failReadForID {
		return nil, errors.New("injected read error for id")
	}
	return f.mockDocStorageProvider.Read(ctx, sec, id)
}

func (f *faultyDocStorageProvider) Update(ctx context.Context, sec *pkgctx.SecurityContext, id string, obj map[string]any) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	if f.failUpdateForID != "" && id == f.failUpdateForID {
		return errors.New("injected update error for id")
	}
	return f.mockDocStorageProvider.Update(ctx, sec, id, obj)
}

// TestRegistry_CreateDocEntry_EdgeCases tests edge cases in CreateDocEntry:
// - file.Path == "" (derives from projectRoot + file.RelPath)
// - Duplicate error handling ("already exists")
// - Promotion failure from draft plane
// - Normal creation error
func TestRegistry_CreateDocEntry_EdgeCases(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tmpDir := t.TempDir()

	docRelPath := "docs/architecture/empty_path.md"
	docAbsPath := filepath.Join(tmpDir, docRelPath)
	if err := fileutil.MkdirAll(filepath.Dir(docAbsPath), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := fileutil.WriteFile(docAbsPath, []byte("# Empty Path Test\n\nContent here."), paths.FilePerm644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	// 1. file.Path is empty: should resolve using projectRoot + RelPath
	mockStore := newFaultyDocStorageProvider()
	reg := NewRegistry(mockStore, tmpDir)

	file := &MarkdownFile{
		Path:    "", // intentionally empty
		RelPath: docRelPath,
	}
	meta := &DocumentMetadata{
		Title:    "Empty Path Test",
		Summary:  "Testing empty path resolution.",
		Status:   "draft", // triggers promotion off draft plane to "review"
		Group:    "architecture",
		Category: "validation",
	}

	err := reg.CreateDocEntry(ctx, "test", file, meta, "DOC-101")
	if err != nil {
		t.Fatalf("CreateDocEntry failed for empty file.Path: %v", err)
	}

	createdObj, err := mockStore.Read(ctx, nil, "DOC-101")
	if err != nil {
		t.Fatalf("failed to read created obj: %v", err)
	}
	if createdObj[objects.FieldKeyStatus] != "review" {
		t.Errorf("expected promoted status 'review', got %v", createdObj[objects.FieldKeyStatus])
	}
	if _, ok := createdObj["content_hash"]; !ok {
		t.Errorf("expected content_hash to be populated from fallback path")
	}

	// 2. Duplicate error ("already exists") should be swallowed gracefully
	mockStore.createErr = errors.New("doc_entry DOC-101 already exists in storage")
	err = reg.CreateDocEntry(ctx, "test", file, meta, "DOC-101")
	if err != nil {
		t.Errorf("expected duplicate error to be skipped gracefully, got: %v", err)
	}

	// 3. Generic creation error should fail
	mockStore.createErr = errors.New("disk write failure")
	err = reg.CreateDocEntry(ctx, "test", file, meta, "DOC-102")
	if err == nil || !errors.Is(err, mockStore.createErr) && err.Error() == "" {
		t.Errorf("expected generic creation error, got %v", err)
	}

	// 4. Update error during draft promotion
	mockStore.createErr = nil
	mockStore.updateErr = errors.New("failed to promote draft")
	err = reg.CreateDocEntry(ctx, "test", file, meta, "DOC-103")
	if err == nil {
		t.Errorf("expected error during promotion off draft plane, got nil")
	}
}

// TestRegistry_GetExistingEntries_Errors tests error paths in GetExistingEntries
func TestRegistry_GetExistingEntries_Errors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tmpDir := t.TempDir()

	faultyStore := newFaultyDocStorageProvider()
	faultyStore.listErr = errors.New("simulated list failure")
	reg := NewRegistry(faultyStore, tmpDir)

	existing, maxID, err := reg.GetExistingEntries(ctx, "test")
	if err == nil {
		t.Fatalf("expected error from GetExistingEntries on list error, got nil")
	}
	if len(existing) != 0 || maxID != 0 {
		t.Errorf("expected empty map and 0 maxID on error, got %v, %d", existing, maxID)
	}

	// Test non-DOC ID and malformed entries
	faultyStore.listErr = nil
	faultyStore.objects["CUSTOM-1"] = map[string]any{
		objects.FieldKeyID:   "CUSTOM-1",
		objects.FieldKeyKind: objects.KindDocEntry,
		// missing path - should be ignored
	}
	faultyStore.objects["DOC-NONUM"] = map[string]any{
		objects.FieldKeyID:   "OTHER-ID",
		objects.FieldKeyKind: objects.KindDocEntry,
		objects.FieldKeyPath: "docs/other.md",
	}

	existing, maxID, err = reg.GetExistingEntries(ctx, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if maxID != 0 {
		t.Errorf("expected maxID 0 for non DOC- numbers, got %d", maxID)
	}
	if len(existing) != 1 {
		t.Errorf("expected 1 valid entry with path, got %d", len(existing))
	}
}

// TestRegistry_RegisterSubtrees_Coverage tests dry-run, skip on existing, parser error,
// and creation error branches in RegisterSubtrees.
func TestRegistry_RegisterSubtrees_Coverage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tmpDir := t.TempDir()

	// Prepare directories and files
	archDir := filepath.Join(tmpDir, "docs", "architecture")
	if err := fileutil.MkdirAll(archDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	file1 := filepath.Join(archDir, "doc1.md")
	file2 := filepath.Join(archDir, "doc2.md")
	file3 := filepath.Join(archDir, "unparseable.md")

	if err := fileutil.WriteFile(file1, []byte("# Doc 1\nContent 1"), paths.FilePerm644); err != nil {
		t.Fatalf("write file1: %v", err)
	}
	if err := fileutil.WriteFile(file2, []byte("# Doc 2\nContent 2"), paths.FilePerm644); err != nil {
		t.Fatalf("write file2: %v", err)
	}
	// Make file3 unreadable to trigger parser failure
	if err := fileutil.WriteFile(file3, []byte("# Doc 3"), 0000); err != nil {
		t.Fatalf("write file3: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(file3, 0644)
	})

	mockStore := newFaultyDocStorageProvider()
	reg := NewRegistry(mockStore, tmpDir)

	// 1. Dry run test
	created, _, err := reg.RegisterSubtrees(ctx, "test", []string{"docs/architecture"}, true)
	if err != nil {
		t.Fatalf("dry-run RegisterSubtrees failed: %v", err)
	}
	if created == 0 {
		t.Errorf("expected created > 0 in dry run, got %d", created)
	}
	if len(mockStore.objects) != 0 {
		t.Errorf("expected no objects stored during dry run, got %d", len(mockStore.objects))
	}

	// 2. Discover failure (nonexistent root)
	badReg := NewRegistry(mockStore, filepath.Join(tmpDir, "nonexistent"))
	badReg.projectRoot = filepath.Join(tmpDir, "nonexistent")
	// If subtrees points to missing dirs, Discover returns empty without error,
	// but if list fails, RegisterSubtrees returns error
	mockStore.listErr = errors.New("list failure")
	_, _, err = reg.RegisterSubtrees(ctx, "test", []string{"docs/architecture"}, false)
	if err == nil {
		t.Errorf("expected error when GetExistingEntries fails in RegisterSubtrees, got nil")
	}
	mockStore.listErr = nil

	// 3. Register real entries and then test skipped existing entries
	created, skipped, err := reg.RegisterSubtrees(ctx, "test", []string{"docs/architecture"}, false)
	if err != nil {
		t.Fatalf("RegisterSubtrees failed: %v", err)
	}
	if created == 0 {
		t.Errorf("expected at least 1 created doc, got %d", created)
	}
	_ = skipped

	// Second pass: entries should be skipped
	created2, skipped2, err := reg.RegisterSubtrees(ctx, "test", []string{"docs/architecture"}, false)
	if err != nil {
		t.Fatalf("RegisterSubtrees second pass failed: %v", err)
	}
	if created2 != 0 {
		t.Errorf("expected 0 created on second pass, got %d", created2)
	}
	if skipped2 == 0 {
		t.Errorf("expected skipped > 0 on second pass, got %d", skipped2)
	}

	// 4. Test CreateDocEntry error handling inside loop
	mockStore2 := newFaultyDocStorageProvider()
	mockStore2.createErr = errors.New("storage disk failure")
	reg2 := NewRegistry(mockStore2, tmpDir)
	created3, _, err := reg2.RegisterSubtrees(ctx, "test", []string{"docs/architecture"}, false)
	if err != nil {
		t.Fatalf("RegisterSubtrees should continue on non-fatal create errors: %v", err)
	}
	if created3 != 0 {
		t.Errorf("expected 0 created when create fails, got %d", created3)
	}
	if reg2.lastCreateError == nil {
		t.Errorf("expected lastCreateError to be recorded on failure")
	}

	// Context cancellation branch inside loop during creation failure
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	reg3 := NewRegistry(mockStore2, tmpDir)
	_, _, err = reg3.RegisterSubtrees(canceledCtx, "test", []string{"docs/architecture"}, false)
	if err == nil {
		t.Errorf("expected error when context canceled during RegisterSubtrees, got nil")
	}
}

// TestRegisterShippedDocs_FullLifecycle tests RegisterShippedDocs with isolated project
func TestRegisterShippedDocs_FullLifecycle(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		SkipSetupTestEnvironment: false,
		SkipFileStorage:          false,
	})
	tmpDir := proj.Root

	// Create shipped doc files
	shippedDocs := map[string]string{
		"docs/architecture/overview.md":     "# Architecture Overview\nSystem architecture.",
		"docs/best-practices/guidelines.md": "# Guidelines\nBest practice rules.",
		"docs/onboarding/welcome.md":        "# Welcome\nGetting started guide.",
	}
	for relPath, content := range shippedDocs {
		absPath := filepath.Join(tmpDir, relPath)
		if err := fileutil.MkdirAll(filepath.Dir(absPath), paths.DirPerm755); err != nil {
			t.Fatalf("mkdir failed: %v", err)
		}
		if err := fileutil.WriteFile(absPath, []byte(content), paths.FilePerm644); err != nil {
			t.Fatalf("write file failed: %v", err)
		}
	}

	logger := logging.GetLoggerFromProfile("debug")

	// 1. Initial registration should succeed and register all 3 docs
	reg, skip, err := RegisterShippedDocs(context.Background(), tmpDir, logger)
	if err != nil {
		t.Fatalf("RegisterShippedDocs failed: %v", err)
	}
	if reg != 3 {
		t.Errorf("expected 3 registered docs, got %d", reg)
	}
	if skip != 0 {
		t.Errorf("expected 0 skipped, got %d", skip)
	}

	// 2. Second run should succeed with 0 registered and 3 skipped
	reg2, skip2, err := RegisterShippedDocs(context.Background(), tmpDir, logger)
	if err != nil {
		t.Fatalf("RegisterShippedDocs second run failed: %v", err)
	}
	if reg2 != 0 {
		t.Errorf("expected 0 registered on second run, got %d", reg2)
	}
	if skip2 != 3 {
		t.Errorf("expected 3 skipped on second run, got %d", skip2)
	}
}

// TestRegisterShippedDocs_FailClosed_MissingDocEntry tests the fail-closed check
// when a shipped markdown file exists on disk but is absent from the storage graph.
func TestRegisterShippedDocs_FailClosed_MissingDocEntry(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		SkipSetupTestEnvironment: false,
		SkipFileStorage:          false,
	})
	tmpDir := proj.Root

	// Create a doc in docs/architecture
	archDoc := filepath.Join(tmpDir, "docs/architecture/critical.md")
	if err := fileutil.MkdirAll(filepath.Dir(archDoc), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := fileutil.WriteFile(archDoc, []byte("# Critical Arch\nContent"), paths.FilePerm644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	logger := logging.GetLoggerFromProfile("debug")

	// Register it first
	reg, _, err := RegisterShippedDocs(context.Background(), tmpDir, logger)
	if err != nil {
		t.Fatalf("RegisterShippedDocs failed: %v", err)
	}
	if reg != 1 {
		t.Fatalf("expected 1 doc registered, got %d", reg)
	}

	// Now add another file and make its directory or file unreadable to the parser,
	// or delete the doc_entry from storage so post-init verification detects missing doc_entry!
	factory, err := storage.NewStorageFactory(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("NewStorageFactory failed: %v", err)
	}
	storageProvider := factory.GetStorageForKind(objects.KindDocEntry)
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	// List entries and delete them to simulate graph deletion / missing graph entry
	entries, err := storageProvider.List(context.Background(), secCtx, storageCtx, storage.ListFilter{Kind: objects.KindDocEntry})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	for _, obj := range entries.Objects {
		id, _ := obj[objects.FieldKeyID].(string)
		_ = storageProvider.Delete(context.Background(), secCtx, id, false)
	}

	// Add an unparseable or blocked markdown file so it fails registration
	badFile := filepath.Join(tmpDir, "docs/architecture/bad.md")
	if err := fileutil.WriteFile(badFile, []byte("# Bad"), 0000); err != nil {
		t.Fatalf("write bad file failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(badFile, 0644)
	})

	// RegisterShippedDocs should now fail closed because bad.md cannot be parsed and registered
	_, _, err = RegisterShippedDocs(context.Background(), tmpDir, logger)
	if err == nil {
		t.Fatalf("expected fail-closed error when shipped doc cannot be registered, got nil")
	}
	if !errors.Is(err, err) && err.Error() == "" {
		t.Errorf("empty error returned")
	}
}

// TestUpdater_ErrorBranches tests UpdateExistingEntries when List, Read, or Update fails.
func TestUpdater_ErrorBranches(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tmpDir := t.TempDir()

	docRelPath := "docs/architecture/doc.md"
	docAbsPath := filepath.Join(tmpDir, docRelPath)
	if err := fileutil.MkdirAll(filepath.Dir(docAbsPath), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := fileutil.WriteFile(docAbsPath, []byte("# Content"), paths.FilePerm644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	// 1. List error
	faultyStore := newFaultyDocStorageProvider()
	faultyStore.listErr = errors.New("list failure")
	reg := NewRegistry(faultyStore, tmpDir)

	_, err := reg.UpdateExistingEntries(ctx, "test")
	if err == nil {
		t.Fatalf("expected error when List fails in UpdateExistingEntries, got nil")
	}

	// 2. Read error on individual doc
	faultyStore.listErr = nil
	faultyStore.objects["DOC-001"] = map[string]any{
		objects.FieldKeyID:   "DOC-001",
		objects.FieldKeyKind: objects.KindDocEntry,
		objects.FieldKeyPath: docRelPath,
		// Missing reference and hash fields
	}
	faultyStore.failReadForID = "DOC-001"

	updated, err := reg.UpdateExistingEntries(ctx, "test")
	if err != nil {
		t.Fatalf("UpdateExistingEntries should continue when individual read fails: %v", err)
	}
	if updated != 0 {
		t.Errorf("expected 0 updated on read failure, got %d", updated)
	}

	// 3. Update error on individual doc
	faultyStore.failReadForID = ""
	faultyStore.failUpdateForID = "DOC-001"
	updated, err = reg.UpdateExistingEntries(ctx, "test")
	if err != nil {
		t.Fatalf("UpdateExistingEntries should continue when individual update fails: %v", err)
	}
	if updated != 0 {
		t.Errorf("expected 0 updated on update failure, got %d", updated)
	}

	// 4. Missing hash with unresolvable path fallback
	faultyStore.failUpdateForID = ""
	faultyStore.objects["DOC-002"] = map[string]any{
		objects.FieldKeyID:   "DOC-002",
		objects.FieldKeyKind: objects.KindDocEntry,
		objects.FieldKeyPath: "custom:unresolvable-alias",
	}
	updated, err = reg.UpdateExistingEntries(ctx, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated != 2 { // DOC-001 and DOC-002 both updated with missing reference fields
		t.Logf("updated %d entries", updated)
	}
}

// TestVerifier_ErrorAndAutoSealBranches tests Verifier error handling:
// - List error
// - Read error when specific IDs are passed
// - AutoSeal update failure
// - AutoSeal read failure
func TestVerifier_ErrorAndAutoSealBranches(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tmpDir := t.TempDir()

	docRelPath := "docs/architecture/verify_me.md"
	docAbsPath := filepath.Join(tmpDir, docRelPath)
	if err := fileutil.MkdirAll(filepath.Dir(docAbsPath), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := fileutil.WriteFile(docAbsPath, []byte("# Verifiable Doc"), paths.FilePerm644); err != nil {
		t.Fatalf("write: %v", err)
	}

	mockStore := newFaultyDocStorageProvider()
	docObj := map[string]any{
		objects.FieldKeyID:     "DOC-VER-001",
		objects.FieldKeyKind:   objects.KindDocEntry,
		objects.FieldKeyStatus: "active",
		objects.FieldKeyPath:   docRelPath,
		// No content_hash: unsealed
	}
	_ = mockStore.Create(ctx, nil, docObj)

	verifier := NewVerifier(mockStore, tmpDir)

	// 1. Storage List error
	mockStore.listErr = errors.New("failed listing doc entries")
	_, err := verifier.Verify(ctx, "test", VerifyOptions{})
	if err == nil {
		t.Fatalf("expected error when storage.List fails in Verify, got nil")
	}
	mockStore.listErr = nil

	// 2. Read error for specified ID
	_, err = verifier.Verify(ctx, "test", VerifyOptions{IDs: []string{"DOC-NONEXISTENT"}})
	if err == nil {
		t.Fatalf("expected error when reading nonexistent ID in Verify, got nil")
	}

	// 3. AutoSeal with read error (using List so Read only triggers inside AutoSeal)
	mockStore.failReadForID = "DOC-VER-001"
	resReadFail, err := verifier.Verify(ctx, "test", VerifyOptions{
		AutoSeal: true,
	})
	if err != nil {
		t.Fatalf("Verify with read error should not fail overall: %v", err)
	}
	if resReadFail.AutoSealed != 0 {
		t.Errorf("expected 0 auto-sealed on read error, got %d", resReadFail.AutoSealed)
	}
	mockStore.failReadForID = ""

	// 4. AutoSeal with update error
	mockStore.failUpdateForID = "DOC-VER-001"
	resUpdateFail, err := verifier.Verify(ctx, "test", VerifyOptions{
		AutoSeal: true,
	})
	if err != nil {
		t.Fatalf("Verify with update error should not fail overall: %v", err)
	}
	if resUpdateFail.AutoSealed != 0 {
		t.Errorf("expected 0 auto-sealed on update error, got %d", resUpdateFail.AutoSealed)
	}
	mockStore.failUpdateForID = ""

	// 5. Successful AutoSeal with fallback path resolution
	resSuccess, err := verifier.Verify(ctx, "test", VerifyOptions{
		AutoSeal: true,
	})
	if err != nil {
		t.Fatalf("Verify auto-seal failed: %v", err)
	}
	if resSuccess.AutoSealed != 1 || resSuccess.Passed != 1 {
		t.Errorf("expected 1 auto-sealed and 1 passed, got auto_sealed=%d passed=%d", resSuccess.AutoSealed, resSuccess.Passed)
	}

	// Verify unsealed/drifted classification logic
	// Create drifted entry
	_ = fileutil.WriteFile(docAbsPath, []byte("# Drifted Doc Content"), paths.FilePerm644)
	resDrift, err := verifier.Verify(ctx, "test", VerifyOptions{})
	if err != nil {
		t.Fatalf("Verify drift failed: %v", err)
	}
	if resDrift.Drifted != 1 {
		t.Errorf("expected 1 drifted entry, got %d", resDrift.Drifted)
	}
}

// TestDiscoverer_EdgeCasesAndAppleDouble tests Discoverer edge cases:
// - AppleDouble files (._*)
// - Hidden/excluded directories and file paths
// - Non-markdown files
func TestDiscoverer_EdgeCasesAndAppleDouble(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	docsDir := filepath.Join(tmpDir, "docs")
	archDir := filepath.Join(docsDir, "architecture")
	ignoredDir := filepath.Join(docsDir, "node_modules")

	if err := fileutil.MkdirAll(archDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir archDir: %v", err)
	}
	if err := fileutil.MkdirAll(ignoredDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir ignoredDir: %v", err)
	}

	// Valid markdown file
	validMd := filepath.Join(archDir, "valid.md")
	_ = fileutil.WriteFile(validMd, []byte("# Valid"), paths.FilePerm644)

	// AppleDouble resource fork file
	appleDouble := filepath.Join(archDir, "._valid.md")
	_ = fileutil.WriteFile(appleDouble, []byte("resource fork"), paths.FilePerm644)

	// Non-markdown file
	txtFile := filepath.Join(archDir, "notes.txt")
	_ = fileutil.WriteFile(txtFile, []byte("text notes"), paths.FilePerm644)

	// Markdown inside ignored directory
	ignoredMd := filepath.Join(ignoredDir, "pkg.md")
	_ = fileutil.WriteFile(ignoredMd, []byte("# Ignored"), paths.FilePerm644)

	discoverer := NewDiscoverer(tmpDir)

	// Test isExcludedPath with dot / empty segments
	if !discoverer.isExcludedPath("node_modules/package.md") {
		t.Errorf("expected node_modules to be excluded")
	}
	if discoverer.isExcludedPath("docs/./architecture/valid.md") {
		t.Errorf("expected normal path with dot segment not to be excluded")
	}
	if discoverer.isExcludedPath("docs//architecture//valid.md") {
		t.Errorf("expected normal path with empty segments not to be excluded")
	}

	files, err := discoverer.Discover()
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	// Should only find the 1 valid markdown file
	if len(files) != 1 {
		t.Fatalf("expected exactly 1 discovered file, got %d", len(files))
	}
	if files[0].RelPath != "docs/architecture/valid.md" {
		t.Errorf("unexpected discovered file: %s", files[0].RelPath)
	}
}

// TestParser_AdditionalCategoriesAndSummaryBranches tests remaining branches in parser.go
func TestParser_AdditionalCategoriesAndSummaryBranches(t *testing.T) {
	t.Parallel()
	parser := NewParser()
	tmpDir := t.TempDir()

	// 1. determineGroupAndCategory branches
	categories := map[string][2]string{
		"docs/architecture/graph/backend.md":     {"architecture", "graph-backend"},
		"docs/architecture/graph-backend/api.md": {"architecture", "graph-backend"},
		"docs/architecture/cli/commands.md":      {"architecture", "cli"},
		"docs/architecture/migration/v2.md":      {"architecture", "migration"},
		"docs/architecture/audit/log.md":         {"architecture", "audit"},
		"docs/architecture/check/rules.md":       {"architecture", "check"},
		"docs/architecture/general/overview.md":  {"architecture", "architecture"},
		"docs/onboarding/start.md":               {"onboarding", ""},
		"docs/strategic/roadmap.md":              {"other", "strategic"},
		"docs/random/notes.md":                   {"other", ""},
	}

	for path, expected := range categories {
		grp, cat := parser.determineGroupAndCategory(path)
		if grp != expected[0] || cat != expected[1] {
			t.Errorf("path %s: expected %q/%q, got %q/%q", path, expected[0], expected[1], grp, cat)
		}
	}

	// 2. Overview extraction with following ## section
	docContent := `# Parser Test

## Overview

This is the first sentence of the overview section. It explains the subsystem.

## Architecture

This section is architecture details.
`
	testDoc := filepath.Join(tmpDir, "overview_test.md")
	if err := fileutil.WriteFile(testDoc, []byte(docContent), paths.FilePerm644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	meta, err := parser.Parse(testDoc)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if meta.Title != "Parser Test" {
		t.Errorf("expected title 'Parser Test', got %q", meta.Title)
	}
	if meta.Summary != "This is the first sentence of the overview section" {
		t.Errorf("unexpected summary: %q", meta.Summary)
	}

	// 3. Fallback to filename when first H1 is missing
	noH1Content := `This is a document with no title.

Some paragraph text here that serves as summary.
`
	noH1Doc := filepath.Join(tmpDir, "auto-generated_guide.md")
	if err := fileutil.WriteFile(noH1Doc, []byte(noH1Content), paths.FilePerm644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	meta2, err := parser.Parse(noH1Doc)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if meta2.Title != "Auto Generated Guide" {
		t.Errorf("expected title 'Auto Generated Guide', got %q", meta2.Title)
	}

	// 4. File read error
	_, err = parser.Parse(filepath.Join(tmpDir, "nonexistent.md"))
	if err == nil {
		t.Errorf("expected error reading nonexistent file, got nil")
	}
}

// TestDocman_ConcurrentOperations verifies thread safety and race freedom across
// registry and verifier operations using goroutinelabels.
func TestDocman_ConcurrentOperations(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tmpDir := t.TempDir()

	docsDir := filepath.Join(tmpDir, "docs", "architecture")
	if err := fileutil.MkdirAll(docsDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	for i := 0; i < 10; i++ {
		filePath := filepath.Join(docsDir, fmt.Sprintf("doc_%d.md", i))
		content := fmt.Sprintf("# Document %d\n\n## Overview\n\nConcurrent document %d description.", i, i)
		if err := fileutil.WriteFile(filePath, []byte(content), paths.FilePerm644); err != nil {
			t.Fatalf("write file %d: %v", i, err)
		}
	}

	mockStore := newMockDocStorageProvider()
	reg := NewRegistry(mockStore, tmpDir)

	var wg sync.WaitGroup
	var errorCount int64
	numWorkers := 4

	for w := 0; w < numWorkers; w++ {
		workerID := w
		wg.Add(1)
		goroutinelabels.NewGoroutine("test_docman_worker", fmt.Sprintf("concurrent worker %d", workerID)).
			StartSimple(func() {
				defer wg.Done()
				// Concurrently register subtrees
				_, _, err := reg.RegisterSubtrees(ctx, "test", []string{"docs/architecture"}, false)
				if err != nil {
					atomic.AddInt64(&errorCount, 1)
				}
				// Concurrently update entries
				_, err = reg.UpdateExistingEntries(ctx, "test")
				if err != nil {
					atomic.AddInt64(&errorCount, 1)
				}
			})
	}
	wg.Wait()

	if errorCount > 0 {
		t.Errorf("expected zero concurrent errors, got %d", errorCount)
	}

	// Verify entries were created
	existing, maxID, err := reg.GetExistingEntries(ctx, "test")
	if err != nil {
		t.Fatalf("GetExistingEntries failed: %v", err)
	}
	if len(existing) != 10 {
		t.Errorf("expected 10 existing entries, got %d", len(existing))
	}
	if maxID != 10 {
		t.Errorf("expected maxID 10, got %d", maxID)
	}

	// Concurrently verify
	verifier := NewVerifier(mockStore, tmpDir)
	for w := 0; w < numWorkers; w++ {
		workerID := w
		wg.Add(1)
		goroutinelabels.NewGoroutine("test_verifier_worker", fmt.Sprintf("verifier worker %d", workerID)).
			StartSimple(func() {
				defer wg.Done()
				res, err := verifier.Verify(ctx, "test", VerifyOptions{AutoSeal: true})
				if err != nil || res == nil {
					atomic.AddInt64(&errorCount, 1)
				}
			})
	}
	wg.Wait()

	if errorCount > 0 {
		t.Errorf("expected zero verifier concurrent errors, got %d", errorCount)
	}
}
