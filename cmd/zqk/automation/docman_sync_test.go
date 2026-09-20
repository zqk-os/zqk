package automation

import (
	"path/filepath"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/docman"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	_ "github.com/zqk-os/zqk/pkg/specbuilder/bldr_v2" // Register spec builders
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func registerDocmanSyncStorageTeardown(t *testing.T, projectRoot string, sp *storage.FileObjectStorage) {
	t.Helper()
	t.Cleanup(func() {
		if err := storage.RunProjectTestTeardown(storage.TempProjectTeardown(projectRoot, sp)); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})
}

func TestDocmanSync_BasicRegistration(t *testing.T) {
	projectRoot := newDocmanSyncTestProject(t)

	// Create a test markdown file
	docsDir := filepath.Join(projectRoot, "docs", "test")
	if err := fileutil.MkdirAll(docsDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create docs dir: %v", err)
	}

	testDoc := filepath.Join(docsDir, "test-doc.md")
	//nolint:gosec // Test files - 0600 is acceptable
	if err := fileutil.WriteSecureFile(testDoc, []byte(`# Test Document

## Overview

This is a test document for docman-sync testing.
`)); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("failed to create test doc: %v", err)
	}

	// Use test storage (SkipGlobalWiring) so global CAS queues are not shared with other
	// parallel tests; otherwise List() can see another test's project root and return 0.
	storageProvider, err := storage.NewFileObjectStorageForTest(projectRoot)
	if err != nil {
		t.Fatalf("failed to create storage provider: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, projectRoot, storageProvider)
	registerDocmanSyncStorageTeardown(t, projectRoot, storageProvider)

	// Create registry
	registry := docman.NewRegistry(storageProvider, projectRoot)

	// Run registration
	ctx := pkgctx.NewSystemContext()
	created, skipped, err := registry.RegisterAll(ctx, automationProfileAIAgent, false)
	if err != nil {
		t.Fatalf("failed to register: %v", err)
	}

	// Verify results
	if created != 1 {
		t.Errorf("expected 1 created, got %d", created)
	}
	if skipped != 0 {
		t.Errorf("expected 0 skipped, got %d", skipped)
	}

	// Verify doc_entry was created
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	filter := storage.ListFilter{Kind: objects.KindDocEntry}
	results, err := storageProvider.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		t.Fatalf("failed to list doc_entries: %v", err)
	}

	if len(results.Objects) != 1 {
		t.Errorf("expected 1 doc_entry, got %d", len(results.Objects))
	}

	// Verify the doc_entry has correct fields (path is stored as path-cache alias per PATH_CACHE_MIGRATION_SCAN.md)
	entry := results.Objects[0]
	expectedPath := paths.PathRefFromRelPath("docs/test/test-doc.md")
	if entry[objects.FieldKeyPath] != expectedPath {
		t.Errorf("expected path %q, got %v", expectedPath, entry[objects.FieldKeyPath])
	}
	if entry[objects.FieldKeyTitle] != "Test Document" {
		t.Errorf("expected title 'Test Document', got %v", entry[objects.FieldKeyTitle])
	}
}

func TestDocmanSync_SkipsExisting(t *testing.T) {
	projectRoot := newDocmanSyncTestProject(t)

	// Create a test markdown file
	docsDir := filepath.Join(projectRoot, "docs", "test")
	if err := fileutil.MkdirAll(docsDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create docs dir: %v", err)
	}

	testDoc := filepath.Join(docsDir, "existing-doc.md")
	//nolint:gosec // Test files - 0600 is acceptable
	if err := fileutil.WriteSecureFile(testDoc, []byte(`# Existing Document

## Overview

This document already has a doc_entry.
`)); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("failed to create test doc: %v", err)
	}

	// Use test storage (SkipGlobalWiring) so global CAS queues are not shared with other
	// parallel tests; otherwise second run can see another test's project root.
	storageProvider, err := storage.NewFileObjectStorageForTest(projectRoot)
	if err != nil {
		t.Fatalf("failed to create storage provider: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, projectRoot, storageProvider)
	registerDocmanSyncStorageTeardown(t, projectRoot, storageProvider)

	// Create registry
	registry := docman.NewRegistry(storageProvider, projectRoot)

	// First registration
	ctx := pkgctx.NewSystemContext()
	created1, _, err := registry.RegisterAll(ctx, automationProfileAIAgent, false)
	if err != nil {
		t.Fatalf("failed to register: %v", err)
	}

	if created1 != 1 {
		t.Errorf("expected 1 created on first run, got %d", created1)
	}

	// Second registration (should skip existing)
	created2, skipped2, err := registry.RegisterAll(ctx, automationProfileAIAgent, false)
	if err != nil {
		t.Fatalf("failed to register second time: %v", err)
	}

	if created2 != 0 {
		t.Errorf("expected 0 created on second run, got %d", created2)
	}
	if skipped2 != 1 {
		t.Errorf("expected 1 skipped on second run, got %d", skipped2)
	}
}

func TestDocmanSync_DryRun(t *testing.T) {
	projectRoot := newDocmanSyncTestProject(t)

	// Create a test markdown file
	docsDir := filepath.Join(projectRoot, "docs", "test")
	if err := fileutil.MkdirAll(docsDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create docs dir: %v", err)
	}

	testDoc := filepath.Join(docsDir, "dry-run-doc.md")
	//nolint:gosec // Test files - 0600 is acceptable
	if err := fileutil.WriteSecureFile(testDoc, []byte(`# Dry Run Document

## Overview

This document is for dry-run testing.
`)); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("failed to create test doc: %v", err)
	}

	// Initialize storage
	storageProvider, err := storage.NewFileObjectStorageForTest(projectRoot)
	if err != nil {
		t.Fatalf("failed to create storage provider: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, projectRoot, storageProvider)
	registerDocmanSyncStorageTeardown(t, projectRoot, storageProvider)

	// Create registry
	registry := docman.NewRegistry(storageProvider, projectRoot)

	// Run dry-run registration
	ctx := pkgctx.NewSystemContext()
	created, _, err := registry.RegisterAll(ctx, automationProfileAIAgent, true)
	if err != nil {
		t.Fatalf("failed to register: %v", err)
	}

	// Verify dry-run reported creation
	if created != 1 {
		t.Errorf("expected 1 would be created, got %d", created)
	}

	// Verify no actual doc_entry was created
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()
	filter := storage.ListFilter{Kind: objects.KindDocEntry}
	results, err := storageProvider.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		t.Fatalf("failed to list doc_entries: %v", err)
	}

	if len(results.Objects) != 0 {
		t.Errorf("expected 0 doc_entries created (dry-run), got %d", len(results.Objects))
	}
}

func TestDocmanSync_DetectMarkdownFiles(t *testing.T) {
	projectRoot := newDocmanSyncTestProject(t)

	// Create test markdown files
	docsDir := filepath.Join(projectRoot, "docs", "test")
	if err := fileutil.MkdirAll(docsDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create docs dir: %v", err)
	}

	testDocs := []string{
		"doc1.md",
		"doc2.md",
		"subdir/doc3.md",
	}

	for _, doc := range testDocs {
		docPath := filepath.Join(docsDir, doc)
		if err := fileutil.MkdirAll(filepath.Dir(docPath), paths.DirPerm755); err != nil {
			t.Fatalf("failed to create subdir: %v", err)
		}
		if err := fileutil.WriteFile(docPath, []byte("# Test Document\n\n## Overview\n\nTest content.\n"), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
			t.Fatalf("failed to create test doc %s: %v", doc, err)
		}
	}

	// Initialize storage
	storageProvider, err := storage.NewFileObjectStorageForTest(projectRoot)
	if err != nil {
		t.Fatalf("failed to create storage provider: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, projectRoot, storageProvider)
	registerDocmanSyncStorageTeardown(t, projectRoot, storageProvider)

	// Create registry
	registry := docman.NewRegistry(storageProvider, projectRoot)

	// Run registration
	ctx := pkgctx.NewSystemContext()
	created, _, err := registry.RegisterAll(ctx, automationProfileAIAgent, false)
	if err != nil {
		t.Fatalf("failed to register: %v", err)
	}

	// Verify all files were discovered and registered
	if created != 3 {
		t.Errorf("expected 3 created, got %d", created)
	}
}

func TestDocmanSync_CheckOnlyMode(t *testing.T) {
	projectRoot := newDocmanSyncTestProject(t)

	// Create a test markdown file
	docsDir := filepath.Join(projectRoot, "docs", "test")
	if err := fileutil.MkdirAll(docsDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create docs dir: %v", err)
	}

	testDoc := filepath.Join(docsDir, "unregistered-doc.md")
	//nolint:gosec // Test files - 0600 is acceptable
	if err := fileutil.WriteSecureFile(testDoc, []byte(`# Unregistered Document

## Overview

This document needs registration.
`)); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("failed to create test doc: %v", err)
	}

	// Initialize storage
	storageProvider, err := storage.NewFileObjectStorageForTest(projectRoot)
	if err != nil {
		t.Fatalf("failed to create storage provider: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, projectRoot, storageProvider)
	registerDocmanSyncStorageTeardown(t, projectRoot, storageProvider)

	// Create registry
	registry := docman.NewRegistry(storageProvider, projectRoot)

	// Check if registration is needed (check-only mode)
	ctx := pkgctx.NewSystemContext()
	created, _, err := registry.RegisterAll(ctx, automationProfileAIAgent, true)
	if err != nil {
		t.Fatalf("failed to check: %v", err)
	}

	// In check-only mode, we should detect that registration is needed
	if created == 0 {
		t.Error("expected to detect that registration is needed (created > 0)")
	}
}

func TestDocmanSync_GitAware_DetectsStagedMarkdown(t *testing.T) {
	projectRoot := newDocmanSyncTestProject(t)

	// Initialize git repo
	if err := execwrap.Command("git", "init", projectRoot).Run(); err != nil {
		t.Skipf("git not available or failed to init: %v", err)
	}

	// Configure git user (required for commits)
	if err := execwrap.Command("git", "-C", projectRoot, "config", "user.name", "Test User").Run(); err != nil {
		t.Fatalf("failed to set git user: %v", err)
	}
	if err := execwrap.Command("git", "-C", projectRoot, "config", "user.email", "test@example.com").Run(); err != nil {
		t.Fatalf("failed to set git email: %v", err)
	}

	// Create a test markdown file
	docsDir := filepath.Join(projectRoot, "docs", "test")
	if err := fileutil.MkdirAll(docsDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create docs dir: %v", err)
	}

	testDoc := filepath.Join(docsDir, "staged-doc.md")
	//nolint:gosec // Test files - 0600 is acceptable
	if err := fileutil.WriteSecureFile(testDoc, []byte(`# Staged Document

## Overview

This document is staged for commit.
`)); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("failed to create test doc: %v", err)
	}

	// Stage the file
	if err := execwrap.Command("git", "-C", projectRoot, "add", testDoc).Run(); err != nil {
		t.Fatalf("failed to stage file: %v", err)
	}

	// Check if markdown files are staged
	hasMarkdown, err := hasStagedMarkdownFiles(projectRoot)
	if err != nil {
		t.Fatalf("failed to check staged markdown: %v", err)
	}

	if !hasMarkdown {
		t.Error("expected to detect staged markdown file")
	}
}

func TestDocmanSync_GitAware_NoStagedMarkdown(t *testing.T) {
	projectRoot := newDocmanSyncTestProject(t)

	// Initialize git repo
	if err := execwrap.Command("git", "init", projectRoot).Run(); err != nil {
		t.Skipf("git not available or failed to init: %v", err)
	}

	// Create a non-markdown file
	testFile := filepath.Join(projectRoot, "test.go")
	if err := fileutil.WriteFile(testFile, []byte("package main\n"), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("failed to create test file: %v", err)
	}

	// Stage the file
	if err := execwrap.Command("git", "-C", projectRoot, "add", testFile).Run(); err != nil {
		t.Fatalf("failed to stage file: %v", err)
	}

	// Check if markdown files are staged
	hasMarkdown, err := hasStagedMarkdownFiles(projectRoot)
	if err != nil {
		t.Fatalf("failed to check staged markdown: %v", err)
	}

	if hasMarkdown {
		t.Error("expected no staged markdown files")
	}
}

// Helper function to setup a test project structure
// newDocmanSyncTestProject allows CAS fallthrough for the test process and lays out an isolated
// project under [testing.T.TempDir], returning its root. Every docman-sync test needs both, so they
// share this instead of repeating the env flag and the temp-project wiring.
func newDocmanSyncTestProject(t *testing.T) string {
	t.Helper()
	t.Setenv(zqkenv.TestAllowCASFallthrough().Name(), "1")
	return setupTestProject(t, t.TempDir())
}

func setupTestProject(t *testing.T, root string) string {
	// Create .zqk directory
	zqkDir := filepath.Join(root, paths.ProjectDataDir)
	if err := fileutil.MkdirAll(zqkDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create .zqk dir: %v", err)
	}

	// Create docs directory
	docsDir := filepath.Join(root, "docs")
	if err := fileutil.MkdirAll(docsDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create docs dir: %v", err)
	}

	// Create process directory structure
	processDir := filepath.Join(docsDir, "process")
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create process dir: %v", err)
	}

	// Create doc_entries directory
	docEntriesDir := filepath.Join(processDir, "doc_entries")
	if err := fileutil.MkdirAll(docEntriesDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create doc_entries dir: %v", err)
	}

	// Create _internal/object_specs directory (required for validation)
	internalDir := filepath.Join(processDir, "_internal", "object_specs")
	if err := fileutil.MkdirAll(internalDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create _internal/object_specs dir: %v", err)
	}

	return root
}
