package git

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestClone_LocalDirectory(t *testing.T) {
	tempDir := t.TempDir()
	sourceRepo := setupTestRepo(t)
	destRepo := filepath.Join(tempDir, "dest")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	createTestCommit(t, sourceRepo, "Initial commit")

	facade, err := CloneToFacade(ctx, sourceRepo, destRepo, CloneOptions{Depth: 1, SingleBranch: true})
	if err != nil {
		t.Fatalf("CloneToFacade failed: %v", err)
	}

	if facade.repoPath != destRepo {
		t.Errorf("expected facade repoPath %s, got %s", destRepo, facade.repoPath)
	}

	branch, err := facade.CurrentBranch()
	if err != nil {
		t.Fatalf("facade.CurrentBranch failed: %v", err)
	}
	if branch == "" {
		t.Errorf("expected non-empty branch")
	}
}

func TestClone_InvalidSource(t *testing.T) {
	tempDir := t.TempDir()
	destRepo := filepath.Join(tempDir, "dest")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := Clone(ctx, "/path/to/nonexistent/repo", destRepo)
	if err == nil {
		t.Fatal("expected error cloning nonexistent repo")
	}
}
