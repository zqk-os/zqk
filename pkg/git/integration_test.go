package git

import (
	"github.com/lanceman/zqk/pkg/datacell"

	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/nildecode"
	"github.com/lanceman/zqk/pkg/paths"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

func registerIntegrationTestStorageTeardown(t *testing.T, testRoot string, sp storage.ObjectStorageProvider) {
	t.Helper()
	fs, ok := nildecode.DecodeNonNilPayload[*storage.FileObjectStorage](sp)
	if !ok {
		return
	}
	t.Cleanup(func() {
		if err := storage.RunProjectTestTeardown(storage.TempProjectTeardown(testRoot, fs)); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})
}

func TestCommitIntegrationService_LinkCommitToWorkItems(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode - creates FileObjectStorage with background goroutines")
	}
	// Setup test repository
	repoPath := setupTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Setup test storage with proper directory structure
	testRoot := t.TempDir()
	ensureObjectSpecsForGitIntegrationTest(t, testRoot)

	storageProvider, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	registerIntegrationTestStorageTeardown(t, testRoot, storageProvider)

	secCtx := pkgctx.NewSystemSecurityContext()
	integrationService := NewCommitIntegrationService(repoPath, storageProvider, secCtx)

	// Create a commit with work item references
	createTestCommit(t, repoPath, "feat: implement ITEM-001 and GOAL-002")

	cmd := exec.Command("git", "log", "-1", "--format=%H")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to get commit hash: %v", err)
	}
	hash := strings.TrimSpace(string(output))

	// Analyze the commit
	analyzer := NewCommitAnalyzer(repoPath)
	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 10*time.Second)
	defer cancel()

	commit, err := analyzer.AnalyzeCommit(ctx, hash)
	if err != nil {
		t.Fatalf("failed to analyze commit: %v", err)
	}

	// Create test work items first
	createTestWorkItem(t, storageProvider, secCtx, "ITEM-001", "backlog_item")
	createTestWorkItem(t, storageProvider, secCtx, "GOAL-002", "goal")

	// Link commit to work items
	ctx2, cancel2 := context.WithTimeout(pkgctx.NewSystemContext(), 30*time.Second)
	defer cancel2()

	err = integrationService.LinkCommitToWorkItems(ctx2, commit)
	if err != nil {
		t.Fatalf("failed to link commit to work items: %v", err)
	}

	// Verify code references were created
	storageCtx := &pkgctx.StorageContext{}
	filter := storage.ListFilter{
		Kind: "code_reference",
		Filters: map[string]any{
			objects.FieldKeyCommitHash: hash,
		},
	}

	result, err := storageProvider.List(ctx2, secCtx, storageCtx, filter)
	if err != nil {
		t.Fatalf("failed to list code references: %v", err)
	}

	if len(result.Objects) == 0 {
		t.Error("expected code references to be created")
	}

	// Verify work items have commit references
	workItem, err := storageProvider.Read(ctx2, secCtx, "ITEM-001")
	if err != nil {
		t.Fatalf("failed to read work item: %v", err)
	}

	commitRefs, ok := workItem[objects.FieldKeyCommitRefs].([]any)
	if !ok || len(commitRefs) == 0 {
		t.Error("expected work item to have commit references")
	}

	found := false
	for _, ref := range commitRefs {
		if refStr, ok := ref.(string); ok && refStr == hash {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected to find commit hash %s in work item commit_refs", hash)
	}
}

func TestCommitIntegrationService_LinkCommitToWorkItems_NonExistentWorkItem(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode - creates FileObjectStorage with background goroutines")
	}
	repoPath := setupTestRepo(t)
	defer os.RemoveAll(repoPath)

	testRoot := t.TempDir()
	ensureObjectSpecsForGitIntegrationTest(t, testRoot)

	storageProvider, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	registerIntegrationTestStorageTeardown(t, testRoot, storageProvider)

	secCtx := pkgctx.NewSystemSecurityContext()
	integrationService := NewCommitIntegrationService(repoPath, storageProvider, secCtx)

	// Create a commit with reference to non-existent work item
	createTestCommit(t, repoPath, "feat: implement ITEM-999")

	cmd := exec.Command("git", "log", "-1", "--format=%H")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to get commit hash: %v", err)
	}
	hash := strings.TrimSpace(string(output))

	analyzer := NewCommitAnalyzer(repoPath)
	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 10*time.Second)
	defer cancel()

	commit, err := analyzer.AnalyzeCommit(ctx, hash)
	if err != nil {
		t.Fatalf("failed to analyze commit: %v", err)
	}

	// Should not error even if work item doesn't exist
	ctx2, cancel2 := context.WithTimeout(pkgctx.NewSystemContext(), 30*time.Second)
	defer cancel2()

	err = integrationService.LinkCommitToWorkItems(ctx2, commit)
	if err != nil {
		t.Fatalf("unexpected error linking to non-existent work item: %v", err)
	}
}

func TestCommitIntegrationService_LinkCommitToWorkItems_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode - creates FileObjectStorage with background goroutines")
	}
	repoPath := setupTestRepo(t)
	defer os.RemoveAll(repoPath)

	testRoot := t.TempDir()
	processDir := datacell.ProcessPrimaryDir(testRoot)
	if err := os.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create process directory: %v", err)
	}
	// Timeout test does not create work items; skip spec copy to avoid TempDir cleanup issues.

	storageProvider, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	registerIntegrationTestStorageTeardown(t, testRoot, storageProvider)

	secCtx := pkgctx.NewSystemSecurityContext()
	integrationService := NewCommitIntegrationService(repoPath, storageProvider, secCtx)
	// Set very short timeout
	integrationService.linkTimeout = 1 * time.Nanosecond

	createTestCommit(t, repoPath, "test commit")

	cmd := exec.Command("git", "log", "-1", "--format=%H")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to get commit hash: %v", err)
	}
	hash := strings.TrimSpace(string(output))

	analyzer := NewCommitAnalyzer(repoPath)
	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 10*time.Second)
	defer cancel()

	commit, err := analyzer.AnalyzeCommit(ctx, hash)
	if err != nil {
		t.Fatalf("failed to analyze commit: %v", err)
	}

	ctx2, cancel2 := context.WithTimeout(pkgctx.NewSystemContext(), 10*time.Second)
	defer cancel2()

	// Should timeout
	err = integrationService.LinkCommitToWorkItems(ctx2, commit)
	if err == nil {
		t.Error("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected timeout error, got: %v", err)
	}
}

func TestCommitIntegrationService_MergeReferences(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode - creates FileObjectStorage with background goroutines")
	}
	repoPath := setupTestRepo(t)
	defer os.RemoveAll(repoPath)

	testRoot := t.TempDir()
	ensureObjectSpecsForGitIntegrationTest(t, testRoot)

	storageProvider, err := storage.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	registerIntegrationTestStorageTeardown(t, testRoot, storageProvider)

	secCtx := pkgctx.NewSystemSecurityContext()
	integrationService := NewCommitIntegrationService(repoPath, storageProvider, secCtx)

	// Create test work item first
	createTestWorkItem(t, storageProvider, secCtx, "ITEM-001", "backlog_item")

	// Create first commit
	createTestCommit(t, repoPath, "feat: implement ITEM-001")
	cmd := exec.Command("git", "log", "-1", "--format=%H")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to get commit hash: %v", err)
	}
	hash1 := strings.TrimSpace(string(output))

	// Create second commit referencing same work item
	createTestCommit(t, repoPath, "fix: update ITEM-001")
	cmd = exec.Command("git", "log", "-1", "--format=%H")
	cmd.Dir = repoPath
	output, err = cmd.Output()
	if err != nil {
		t.Fatalf("failed to get commit hash: %v", err)
	}
	hash2 := strings.TrimSpace(string(output))

	analyzer := NewCommitAnalyzer(repoPath)
	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 10*time.Second)
	defer cancel()

	commit1, err := analyzer.AnalyzeCommit(ctx, hash1)
	if err != nil {
		t.Fatalf("failed to analyze commit: %v", err)
	}

	commit2, err := analyzer.AnalyzeCommit(ctx, hash2)
	if err != nil {
		t.Fatalf("failed to analyze commit: %v", err)
	}

	// Link both commits
	ctx2, cancel2 := context.WithTimeout(pkgctx.NewSystemContext(), 30*time.Second)
	defer cancel2()

	if err := integrationService.LinkCommitToWorkItems(ctx2, commit1); err != nil {
		t.Fatalf("failed to link first commit: %v", err)
	}

	if err := integrationService.LinkCommitToWorkItems(ctx2, commit2); err != nil {
		t.Fatalf("failed to link second commit: %v", err)
	}

	// Verify work item has both commit references
	workItem, err := storageProvider.Read(ctx2, secCtx, "ITEM-001")
	if err != nil {
		t.Fatalf("failed to read work item: %v", err)
	}

	commitRefs, ok := workItem[objects.FieldKeyCommitRefs].([]any)
	if !ok {
		t.Fatal("commit_refs is not a list")
	}

	if len(commitRefs) != 2 {
		t.Errorf("expected 2 commit references, got %d", len(commitRefs))
	}

	// Verify both hashes are present
	found1 := false
	found2 := false
	for _, ref := range commitRefs {
		if refStr, ok := ref.(string); ok {
			if refStr == hash1 {
				found1 = true
			}
			if refStr == hash2 {
				found2 = true
			}
		}
	}

	if !found1 {
		t.Error("expected to find first commit hash")
	}
	if !found2 {
		t.Error("expected to find second commit hash")
	}
}

// createTestWorkItem creates a test work item in storage
func createTestWorkItem(t *testing.T, storageProvider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, id, kind string) {
	workItem := map[string]any{
		objects.FieldKeyID:            id,
		objects.FieldKeyKind:          kind,
		objects.FieldKeyTitle:         fmt.Sprintf("Fixture: %s", id),
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	// Set appropriate status based on kind
	switch kind {
	case "backlog_item":
		workItem[objects.FieldKeyStatus] = "exploring"
	case "milestone":
		workItem[objects.FieldKeyStatus] = "planned"
	case "goal":
		workItem[objects.FieldKeyStatus] = "active"
	default:
		workItem[objects.FieldKeyStatus] = "active"
	}

	ctx := pkgctx.NewSystemContext()
	if err := storageProvider.Create(ctx, secCtx, workItem); err != nil {
		t.Fatalf("failed to create work item %s: %v", id, err)
	}
}
