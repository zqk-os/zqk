package paths

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestValidateProjectRootNesting_CleanRootPasses(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Initialize top-level root
	topData := filepath.Join(tmpDir, ProjectDataDir)
	if err := fileutil.MkdirAll(topData, DirPerm755); err != nil {
		t.Fatalf("failed to create top data dir: %v", err)
	}

	if err := ValidateProjectRootNesting(tmpDir); err != nil {
		t.Fatalf("expected valid standalone project root to pass nesting check, got: %v", err)
	}
}

func TestValidateProjectRootNesting_AncestorViolationFails(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Top-level root has .zqk
	topData := filepath.Join(tmpDir, ProjectDataDir)
	if err := fileutil.MkdirAll(topData, DirPerm755); err != nil {
		t.Fatalf("failed to create top data dir: %v", err)
	}

	// Subdirectory attempts to be a project root
	subDir := filepath.Join(tmpDir, "subpkg", "nested")
	subData := filepath.Join(subDir, ProjectDataDir)
	if err := fileutil.MkdirAll(subData, DirPerm755); err != nil {
		t.Fatalf("failed to create sub data dir: %v", err)
	}

	err := ValidateProjectRootNesting(subDir)
	if err == nil {
		t.Fatalf("expected ancestor violation error for nested root, got nil")
	}
	if !errors.Is(err, ErrNestedProjectRoot) {
		t.Fatalf("expected ErrNestedProjectRoot, got: %v", err)
	}
}

func TestValidateProjectRootNesting_DescendantViolationFails(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Top-level root has .zqk
	topData := filepath.Join(tmpDir, ProjectDataDir)
	if err := fileutil.MkdirAll(topData, DirPerm755); err != nil {
		t.Fatalf("failed to create top data dir: %v", err)
	}

	// Rogue nested .zqk created in subfolder
	subDir := filepath.Join(tmpDir, "pkg", "core")
	subData := filepath.Join(subDir, ProjectDataDir)
	if err := fileutil.MkdirAll(subData, DirPerm755); err != nil {
		t.Fatalf("failed to create nested sub data dir: %v", err)
	}

	// Checking top-level root should catch the enclosed descendant violation
	err := ValidateProjectRootNesting(tmpDir)
	if err == nil {
		t.Fatalf("expected descendant violation error for root containing nested project root, got nil")
	}
	if !errors.Is(err, ErrNestedProjectRoot) {
		t.Fatalf("expected ErrNestedProjectRoot, got: %v", err)
	}

	// Purge should remove the nested .zqk
	purged, err := PurgeNestedProjectRoots(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error purging nested roots: %v", err)
	}
	if len(purged) != 1 {
		t.Fatalf("expected 1 purged nested root, got %d", len(purged))
	}

	// After purge, validation should pass
	if err := ValidateProjectRootNesting(tmpDir); err != nil {
		t.Fatalf("expected valid project root after purge, got: %v", err)
	}
}
