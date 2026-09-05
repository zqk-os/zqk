package fileutil

import (
	"path/filepath"
	"testing"
)

func TestFileUtil_IOPS(t *testing.T) {
	dir := t.TempDir()

	// Test EnsureDir
	subDir := filepath.Join(dir, "sub")
	if err := EnsureDir(subDir); err != nil {
		t.Fatalf("EnsureDir failed: %v", err)
	}

	// Test WriteStandardFile & ReadFile
	stdPath := filepath.Join(subDir, "std.txt")
	content := []byte("hello fileutil")
	if err := WriteStandardFile(stdPath, content); err != nil {
		t.Fatalf("WriteStandardFile failed: %v", err)
	}

	got, err := ReadFile(stdPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("ReadFile got %s, want %s", string(got), string(content))
	}
	stdInfo, err := Stat(stdPath)
	if err != nil {
		t.Fatalf("stat standard file: %v", err)
	}
	if perm := stdInfo.Mode().Perm(); perm != standardFilePerm {
		t.Errorf("WriteStandardFile perm got %o, want %o", perm, standardFilePerm)
	}

	// Test WriteSecureFile
	secPath := filepath.Join(subDir, "sec.txt")
	if err := WriteSecureFile(secPath, content); err != nil {
		t.Fatalf("WriteSecureFile failed: %v", err)
	}

	stat, err := Stat(secPath)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if perm := stat.Mode().Perm(); perm != 0600 {
		t.Errorf("WriteSecureFile perm got %o, want 0600", perm)
	}

	if !Exists(stdPath) || !IsRegularFile(stdPath) {
		t.Fatal("expected Exists/IsRegularFile for standard file")
	}
	if size, ok := FileSize(stdPath); !ok || size != int64(len(content)) {
		t.Fatalf("FileSize=%d ok=%v", size, ok)
	}
	exePath := filepath.Join(subDir, "run.sh")
	if err := WriteExecutableFile(exePath, []byte("#!/bin/sh\n")); err != nil {
		t.Fatalf("WriteExecutableFile: %v", err)
	}
	st, err := Stat(exePath)
	if err != nil {
		t.Fatal(err)
	}
	if perm := st.Mode().Perm(); perm != executableFilePerm {
		t.Fatalf("executable perm got %o, want %o", perm, executableFilePerm)
	}
	copyPath := filepath.Join(subDir, "copy.sh")
	if err := CopyExecutableFile(exePath, copyPath); err != nil {
		t.Fatalf("CopyExecutableFile: %v", err)
	}
	copyInfo, err := Stat(copyPath)
	if err != nil {
		t.Fatal(err)
	}
	if perm := copyInfo.Mode().Perm(); perm != executableFilePerm {
		t.Fatalf("copied executable perm got %o, want %o", perm, executableFilePerm)
	}
	disabledPath := copyPath + ".disabled"
	if err := RenameFile(copyPath, disabledPath); err != nil {
		t.Fatalf("RenameFile: %v", err)
	}
	if err := Chmod(disabledPath, secureFilePerm); err != nil {
		t.Fatalf("make copied file non-executable: %v", err)
	}
	if err := EnsureExecutable(disabledPath); err != nil {
		t.Fatalf("EnsureExecutable: %v", err)
	}
	enabledInfo, err := Stat(disabledPath)
	if err != nil {
		t.Fatal(err)
	}
	if perm := enabledInfo.Mode().Perm(); perm != executableFilePerm {
		t.Fatalf("EnsureExecutable perm got %o, want %o", perm, executableFilePerm)
	}
	f, err := OpenAppend(stdPath)
	if err != nil {
		t.Fatalf("OpenAppend: %v", err)
	}
	_, _ = f.WriteString("\nmore")
	_ = f.Close()

	readF, err := OpenRead(stdPath)
	if err != nil {
		t.Fatalf("OpenRead: %v", err)
	}
	_ = readF.Close()
	truncPath := filepath.Join(subDir, "trunc.txt")
	tf, err := OpenSecureTrunc(truncPath)
	if err != nil {
		t.Fatalf("OpenSecureTrunc: %v", err)
	}
	if _, err := tf.WriteString("secure"); err != nil {
		t.Fatalf("OpenSecureTrunc write: %v", err)
	}
	_ = tf.Close()
	truncInfo, err := Stat(truncPath)
	if err != nil {
		t.Fatal(err)
	}
	if perm := truncInfo.Mode().Perm(); perm != secureFilePerm {
		t.Fatalf("OpenSecureTrunc perm got %o, want %o", perm, secureFilePerm)
	}

	removePath := filepath.Join(subDir, "remove-me.txt")
	if err := WriteStandardFile(removePath, []byte("gone")); err != nil {
		t.Fatalf("WriteStandardFile for RemoveFile: %v", err)
	}
	if err := RemoveFile(removePath); err != nil {
		t.Fatalf("RemoveFile: %v", err)
	}
	if Exists(removePath) {
		t.Fatal("RemoveFile left the path behind")
	}
}

func TestFileUtil_DurableWrite(t *testing.T) {
	dir := t.TempDir()
	durablePath := filepath.Join(dir, "durable.txt")
	content := []byte("durable fsync test payload")

	if err := WriteDurableStandardFile(durablePath, content); err != nil {
		t.Fatalf("WriteDurableStandardFile failed: %v", err)
	}

	got, err := ReadFile(durablePath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("ReadFile got %s, want %s", string(got), string(content))
	}

	st, err := Stat(durablePath)
	if err != nil {
		t.Fatal(err)
	}
	if perm := st.Mode().Perm(); perm != standardFilePerm {
		t.Errorf("WriteDurableStandardFile perm got %o, want %o", perm, standardFilePerm)
	}

	// Test secure durable write
	secDurablePath := filepath.Join(dir, "sec_durable.txt")
	if err := WriteDurableSecureFile(secDurablePath, content); err != nil {
		t.Fatalf("WriteDurableSecureFile failed: %v", err)
	}
	secSt, err := Stat(secDurablePath)
	if err != nil {
		t.Fatal(err)
	}
	if perm := secSt.Mode().Perm(); perm != secureFilePerm {
		t.Errorf("WriteDurableSecureFile perm got %o, want %o", perm, secureFilePerm)
	}
}
