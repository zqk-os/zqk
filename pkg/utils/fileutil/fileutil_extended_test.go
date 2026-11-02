package fileutil_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestFileutilCoverageElevation(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("WithOpenFile", func(t *testing.T) {
		filePath := filepath.Join(tmpDir, "with_open.txt")
		if err := fileutil.WriteStandardFile(filePath, []byte("hello world")); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		called := false
		err := fileutil.WithOpenFile(filePath, func(f *os.File) error {
			called = true
			buf := make([]byte, 5)
			n, err := f.Read(buf)
			if err != nil || n != 5 || string(buf) != "hello" {
				t.Fatalf("unexpected read: %v, string=%s", err, string(buf))
			}
			return nil
		})
		if err != nil || !called {
			t.Fatalf("WithOpenFile failed: err=%v, called=%v", err, called)
		}

		customErr := errors.New("custom error")
		err = fileutil.WithOpenFile(filePath, func(f *os.File) error {
			return customErr
		})
		if !errors.Is(err, customErr) {
			t.Fatalf("expected customErr, got %v", err)
		}

		nonExistent := filepath.Join(tmpDir, "missing.txt")
		err = fileutil.WithOpenFile(nonExistent, func(f *os.File) error {
			return nil
		})
		if err == nil {
			t.Fatalf("expected error opening non-existent file")
		}
	})

	t.Run("ReadPIDFile", func(t *testing.T) {
		missingPath := filepath.Join(tmpDir, "missing.pid")
		if pid, ok := fileutil.ReadPIDFile(missingPath); ok || pid != 0 {
			t.Fatalf("expected (0, false) for missing file, got (%d, %v)", pid, ok)
		}

		nanPath := filepath.Join(tmpDir, "nan.pid")
		if err := fileutil.WriteStandardFile(nanPath, []byte("abc\n")); err != nil {
			t.Fatalf("failed to write nan.pid: %v", err)
		}
		if pid, ok := fileutil.ReadPIDFile(nanPath); ok || pid != 0 {
			t.Fatalf("expected (0, false) for NaN pid, got (%d, %v)", pid, ok)
		}

		zeroPath := filepath.Join(tmpDir, "zero.pid")
		if err := fileutil.WriteStandardFile(zeroPath, []byte("0\n")); err != nil {
			t.Fatalf("failed to write zero.pid: %v", err)
		}
		if pid, ok := fileutil.ReadPIDFile(zeroPath); ok || pid != 0 {
			t.Fatalf("expected (0, false) for 0 pid, got (%d, %v)", pid, ok)
		}

		validPath := filepath.Join(tmpDir, "valid.pid")
		if err := fileutil.WriteStandardFile(validPath, []byte(" 54321 \n")); err != nil {
			t.Fatalf("failed to write valid.pid: %v", err)
		}
		if pid, ok := fileutil.ReadPIDFile(validPath); !ok || pid != 54321 {
			t.Fatalf("expected (54321, true), got (%d, %v)", pid, ok)
		}
	})

	t.Run("IgnoreNotExist", func(t *testing.T) {
		if err := fileutil.IgnoreNotExist(nil); err != nil {
			t.Fatalf("expected nil for nil err, got %v", err)
		}
		if err := fileutil.IgnoreNotExist(os.ErrNotExist); err != nil {
			t.Fatalf("expected nil for ErrNotExist, got %v", err)
		}
		sentinel := errors.New("sentinel")
		if err := fileutil.IgnoreNotExist(sentinel); !errors.Is(err, sentinel) {
			t.Fatalf("expected sentinel err, got %v", err)
		}
	})

	t.Run("EnsureSymlink", func(t *testing.T) {
		targetFile := filepath.Join(tmpDir, "target_bin")
		if err := fileutil.WriteExecutableFile(targetFile, []byte("#!/bin/sh\necho test\n")); err != nil {
			t.Fatalf("failed to write target_bin: %v", err)
		}

		linkPath := filepath.Join(tmpDir, "bin", "target_symlink")
		absLink, err := fileutil.EnsureSymlink(linkPath, targetFile)
		if err != nil {
			t.Fatalf("EnsureSymlink failed: %v", err)
		}
		if absLink == "" {
			t.Fatalf("expected non-empty link path")
		}

		absLink2, err2 := fileutil.EnsureSymlink(linkPath, targetFile)
		if err2 != nil || absLink2 != absLink {
			t.Fatalf("idempotent EnsureSymlink failed: %v, got %s, expected %s", err2, absLink2, absLink)
		}

		regularFile := filepath.Join(tmpDir, "regular_file")
		if err := fileutil.WriteStandardFile(regularFile, []byte("content")); err != nil {
			t.Fatalf("failed to write regular_file: %v", err)
		}
		res, err := fileutil.EnsureSymlink(regularFile, targetFile)
		if err != nil || res != regularFile {
			t.Fatalf("expected regular file returned unchanged, got %s, %v", res, err)
		}

		newTarget := filepath.Join(tmpDir, "target_bin_v2")
		if err := fileutil.WriteExecutableFile(newTarget, []byte("#!/bin/sh\necho v2\n")); err != nil {
			t.Fatalf("failed to write target_bin_v2: %v", err)
		}
		absLink3, err3 := fileutil.EnsureSymlink(linkPath, newTarget)
		if err3 != nil || absLink3 == "" {
			t.Fatalf("failed to re-link to newTarget: %v", err3)
		}
	})

	t.Run("IsExist_IsPermission", func(t *testing.T) {
		if !fileutil.IsExist(os.ErrExist) {
			t.Fatalf("IsExist should return true for os.ErrExist")
		}
		if fileutil.IsExist(os.ErrNotExist) {
			t.Fatalf("IsExist should return false for os.ErrNotExist")
		}
		if !fileutil.IsPermission(os.ErrPermission) {
			t.Fatalf("IsPermission should return true for os.ErrPermission")
		}
		if fileutil.IsPermission(nil) {
			t.Fatalf("IsPermission should return false for nil")
		}
	})

	t.Run("FS_Wrappers_And_JSON", func(t *testing.T) {
		jsonPath := filepath.Join(tmpDir, "data.json")
		data := map[string]string{"foo": "bar"}
		if err := fileutil.WriteSecureJSONIndent(jsonPath, data); err != nil {
			t.Fatalf("WriteSecureJSONIndent failed: %v", err)
		}

		linesPath := filepath.Join(tmpDir, "lines.jsonl")
		if err := fileutil.AppendJSONLine(linesPath, map[string]int{"a": 1}); err != nil {
			t.Fatalf("AppendJSONLine 1 failed: %v", err)
		}
		if err := fileutil.AppendJSONLine(linesPath, map[string]int{"b": 2}); err != nil {
			t.Fatalf("AppendJSONLine 2 failed: %v", err)
		}

		createdPath := filepath.Join(tmpDir, "created.txt")
		f, err := fileutil.Create(createdPath)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		_ = f.Close()

		if err := fileutil.Truncate(createdPath, 0); err != nil {
			t.Fatalf("Truncate failed: %v", err)
		}

		linkPath := filepath.Join(tmpDir, "hardlink.txt")
		if err := fileutil.Link(createdPath, linkPath); err != nil {
			t.Fatalf("Link failed: %v", err)
		}

		symPath := filepath.Join(tmpDir, "symlink.txt")
		if err := fileutil.Symlink(createdPath, symPath); err != nil {
			t.Fatalf("Symlink failed: %v", err)
		}
		target, err := fileutil.Readlink(symPath)
		if err != nil || target != createdPath {
			t.Fatalf("Readlink failed: %v, got %s", err, target)
		}

		if err := fileutil.RemoveFileIfExists(linkPath); err != nil {
			t.Fatalf("RemoveFileIfExists failed: %v", err)
		}
		if err := fileutil.RemoveFileIfExists(linkPath); err != nil {
			t.Fatalf("RemoveFileIfExists on non-existent failed: %v", err)
		}

		renamedPath := filepath.Join(tmpDir, "renamed.txt")
		if err := fileutil.Rename(createdPath, renamedPath); err != nil {
			t.Fatalf("Rename failed: %v", err)
		}
		if !fileutil.Exists(renamedPath) {
			t.Fatalf("renamedPath does not exist")
		}
	})

	t.Run("Empty_Name_Negative_Boundaries", func(t *testing.T) {
		_, _ = fileutil.Create("")
		_ = fileutil.Chmod("", 0600)
		_ = fileutil.Chtimes("", time.Time{}, time.Time{})
		_ = fileutil.Truncate("", 0)
		_ = fileutil.Link("", "")
		_ = fileutil.Symlink("", "")
		_, _ = fileutil.Readlink("")
		_, _ = fileutil.Open("")
		_, _ = fileutil.OpenFile("", os.O_RDONLY, 0)
		_ = fileutil.Remove("")
		_ = fileutil.RemoveAll("")
		_, _ = fileutil.Lstat("")
		_, _ = fileutil.Stat("")
		_, _ = fileutil.ReadDir("")
		_ = fileutil.Mkdir("", 0755)
		_ = fileutil.MkdirAll("", 0755)

		// Path safety violations (null byte)
		invalidPath := "bad\x00path"
		if _, err := fileutil.Create(invalidPath); err == nil {
			t.Fatalf("expected error for invalid path in Create")
		}
		if err := fileutil.Chmod(invalidPath, 0644); err == nil {
			t.Fatalf("expected error for invalid path in Chmod")
		}
		if err := fileutil.Truncate(invalidPath, 0); err == nil {
			t.Fatalf("expected error for invalid path in Truncate")
		}
		if _, err := fileutil.Open(invalidPath); err == nil {
			t.Fatalf("expected error for invalid path in Open")
		}
		if err := fileutil.Remove(invalidPath); err == nil {
			t.Fatalf("expected error for invalid path in Remove")
		}
	})

	t.Run("Durable_And_Open_Modes", func(t *testing.T) {
		durablePath := filepath.Join(tmpDir, "durable.txt")
		if err := fileutil.WriteDurableStandardFile(durablePath, []byte("durable-std")); err != nil {
			t.Fatalf("WriteDurableStandardFile failed: %v", err)
		}
		if err := fileutil.WriteDurableSecureFile(durablePath, []byte("durable-sec")); err != nil {
			t.Fatalf("WriteDurableSecureFile failed: %v", err)
		}
		if err := fileutil.WriteDurableExecutableFile(durablePath, []byte("#!/bin/sh\n")); err != nil {
			t.Fatalf("WriteDurableExecutableFile failed: %v", err)
		}

		truncPath := filepath.Join(tmpDir, "trunc.txt")
		f1, err := fileutil.OpenSecureTrunc(truncPath)
		if err != nil {
			t.Fatalf("OpenSecureTrunc failed: %v", err)
		}
		_ = f1.Close()

		f2, err := fileutil.OpenAppend(truncPath)
		if err != nil {
			t.Fatalf("OpenAppend failed: %v", err)
		}
		_ = f2.Close()

		if fileutil.Exists("/non/existent/path/for/sure/12345") {
			t.Fatalf("expected Exists to return false")
		}
	})

	t.Run("ValidateNoRepoStateMutation", func(t *testing.T) {
		if err := fileutil.ValidateNoRepoStateMutation("", "/repo", "/test"); err != nil {
			t.Fatalf("expected nil for empty target: %v", err)
		}
		if err := fileutil.ValidateNoRepoStateMutation("/target", "", "/test"); err != nil {
			t.Fatalf("expected nil for empty repo: %v", err)
		}
		repoDir := tmpDir
		outsideDir := filepath.Join(tmpDir, "outside")
		if err := fileutil.ValidateNoRepoStateMutation(outsideDir, filepath.Join(repoDir, "repo"), filepath.Join(repoDir, "test")); err != nil {
			t.Fatalf("expected nil for outside dir: %v", err)
		}

		protectedTarget := filepath.Join(repoDir, ".zqk", "process", "items.yaml")
		err := fileutil.ValidateNoRepoStateMutation(protectedTarget, repoDir, filepath.Join(tmpDir, "subtest"))
		if err == nil {
			t.Fatalf("expected error for protected repo mutation")
		}
	})

	t.Run("ObjectYAMLIO", func(t *testing.T) {
		limit := fileutil.ObjectYAMLIOLimit()
		if limit <= 0 {
			t.Fatalf("expected positive ObjectYAMLIOLimit, got %d", limit)
		}
		sampleFile := filepath.Join(tmpDir, "sample_yaml.yaml")
		if err := fileutil.WriteStandardFile(sampleFile, []byte("key: value\n")); err != nil {
			t.Fatalf("failed to write sample_yaml: %v", err)
		}
		data, err := fileutil.ReadFileGated(sampleFile)
		if err != nil || string(data) != "key: value\n" {
			t.Fatalf("ReadFileGated failed: %v, got %s", err, string(data))
		}
	})
}
