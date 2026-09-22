package fileutil

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExtraCoverage_FS_Helpers(t *testing.T) {
	// Error inspectors
	if !IsNotExist(os.ErrNotExist) {
		t.Error("expected IsNotExist true")
	}
	if !IsExist(os.ErrExist) {
		t.Error("expected IsExist true")
	}
	if !IsPermission(os.ErrPermission) {
		t.Error("expected IsPermission true")
	}

	tmpDir := t.TempDir()

	// MkdirTemp
	createdDir, err := MkdirTemp(tmpDir, "test-dir-*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	if !Exists(createdDir) {
		t.Errorf("expected createdDir to exist")
	}

	// Create
	testFilePath := filepath.Join(createdDir, "create.txt")
	createdF, err := Create(testFilePath)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	// TermFdInt
	fdInt := TermFdInt(createdF)
	if fdInt < 0 {
		t.Errorf("expected valid fd int, got %d", fdInt)
	}
	_ = createdF.Close()

	// Chtimes
	now := time.Now().Add(-1 * time.Hour)
	if err := Chtimes(testFilePath, now, now); err != nil {
		t.Errorf("Chtimes failed: %v", err)
	}

	// Truncate
	if err := Truncate(testFilePath, 10); err != nil {
		t.Errorf("Truncate failed: %v", err)
	}
	if size, ok := FileSize(testFilePath); !ok || size != 10 {
		t.Errorf("expected size 10 after Truncate, got %d (ok=%v)", size, ok)
	}

	// Link & Symlink & Readlink & Lstat
	linkPath := filepath.Join(createdDir, "link.txt")
	if err := Link(testFilePath, linkPath); err != nil {
		t.Errorf("Link failed: %v", err)
	}
	symlinkPath := filepath.Join(createdDir, "symlink.txt")
	if err := Symlink(testFilePath, symlinkPath); err != nil {
		t.Errorf("Symlink failed: %v", err)
	}
	target, err := Readlink(symlinkPath)
	if err != nil || target != testFilePath {
		t.Errorf("Readlink failed: target=%s, err=%v", target, err)
	}
	lst, err := Lstat(symlinkPath)
	if err != nil || lst.Mode()&os.ModeSymlink == 0 {
		t.Errorf("Lstat failed or not symlink: %v", err)
	}

	// RemoveAll & RemoveFileIfExists
	if err := RemoveFileIfExists(linkPath); err != nil {
		t.Errorf("RemoveFileIfExists failed: %v", err)
	}
	if err := RemoveFileIfExists(filepath.Join(createdDir, "nonexistent")); err != nil {
		t.Errorf("RemoveFileIfExists on nonexistent returned error: %v", err)
	}
	if err := RemoveAll(createdDir); err != nil {
		t.Errorf("RemoveAll failed: %v", err)
	}

	// Getwd & UserHomeDir & TempDir & Executable
	if wd, err := Getwd(); err != nil || wd == "" {
		t.Errorf("Getwd failed: %v", err)
	}
	if home, err := UserHomeDir(); err != nil || home == "" {
		t.Errorf("UserHomeDir failed: %v", err)
	}
	if td := TempDir(); td == "" {
		t.Error("TempDir returned empty")
	}
	if exe, err := Executable(); err != nil || exe == "" {
		t.Errorf("Executable failed: %v", err)
	}

	// SyncDir
	syncDir := t.TempDir()
	if err := SyncDir(syncDir); err != nil {
		t.Errorf("SyncDir failed: %v", err)
	}
	if err := SyncDir(filepath.Join(syncDir, "nonexistent")); err == nil {
		t.Error("SyncDir expected error on nonexistent dir")
	}
}

func TestExtraCoverage_FindFiles(t *testing.T) {
	tmpDir := t.TempDir()
	known := map[string]string{
		"mcp.json":  "mcp_config",
		"tool.yaml": "tool_spec",
	}

	// Setup hierarchy
	subDir := filepath.Join(tmpDir, "level1", "level2")
	if err := MkdirAll(subDir, StandardDirPerm); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := WriteStandardFile(filepath.Join(subDir, "mcp.json"), []byte("{}")); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	// Setup skip dir
	gitDir := filepath.Join(tmpDir, ".git")
	if err := MkdirAll(gitDir, StandardDirPerm); err != nil {
		t.Fatalf("MkdirAll gitDir failed: %v", err)
	}
	if err := WriteStandardFile(filepath.Join(gitDir, "tool.yaml"), []byte("tool")); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	matches := FindFiles([]string{tmpDir, tmpDir, filepath.Join(tmpDir, "nonexistent")}, known)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match (excluding .git and deduping searchDirs), got %d: %+v", len(matches), matches)
	}
	if matches[0].Identifier != "mcp_config" {
		t.Errorf("expected identifier mcp_config, got %s", matches[0].Identifier)
	}
}

func TestExtraCoverage_IOPS_EdgeCases(t *testing.T) {
	tmpDir := t.TempDir()

	// Non-existent FileSize
	if _, ok := FileSize(filepath.Join(tmpDir, "none")); ok {
		t.Error("expected FileSize false for nonexistent")
	}

	// OpenSecureTrunc with invalid path
	if _, err := OpenSecureTrunc(filepath.Join(tmpDir, "missing", "bad.txt")); err == nil {
		t.Error("expected error for OpenSecureTrunc with missing parent")
	}

	// OpenAppend with invalid path
	if _, err := OpenAppend(filepath.Join(tmpDir, "missing", "bad.txt")); err == nil {
		t.Error("expected error for OpenAppend with missing parent")
	}

	// CopyExecutableFile with nonexistent source
	if err := CopyExecutableFile(filepath.Join(tmpDir, "none"), filepath.Join(tmpDir, "dst")); err == nil {
		t.Error("expected error for CopyExecutableFile nonexistent source")
	}

	// Chdir
	origWd, _ := Getwd()
	if err := Chdir(tmpDir); err != nil {
		t.Errorf("Chdir failed: %v", err)
	}
	_ = Chdir(origWd)
}
