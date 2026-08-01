package objects

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestNewFileSpecStorageProvider_ReadSpecBytes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	specDir := filepath.Join(dir, "object_specs")
	if err := fileutil.EnsureDir(specDir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(specDir, "sample.yaml")
	content := []byte("ontology: sample\n")
	if err := fileutil.WriteSecureFile(path, content); err != nil {
		t.Fatal(err)
	}

	p, err := NewFileSpecStorageProvider(specDir)
	if err != nil {
		t.Fatalf("NewFileSpecStorageProvider: %v", err)
	}
	data, meta, err := p.ReadSpecBytes(context.Background(), path)
	if err != nil {
		t.Fatalf("ReadSpecBytes: %v", err)
	}
	if !bytes.Equal(data, content) {
		t.Fatalf("data mismatch: %q", data)
	}
	if meta.ModTime.IsZero() {
		t.Fatal("expected non-zero ModTime")
	}
}

func TestFileSpecStorageProvider_ReadOutsideRootRejected(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	specDir := filepath.Join(dir, "object_specs")
	if err := fileutil.EnsureDir(specDir); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(specDir, "in.yaml")
	if err := fileutil.WriteSecureFile(inside, []byte("ontology: in\n")); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(dir, "outside.yaml")
	if err := fileutil.WriteSecureFile(outside, []byte("x")); err != nil {
		t.Fatal(err)
	}

	p, err := NewFileSpecStorageProvider(specDir)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = p.ReadSpecBytes(context.Background(), outside)
	if err == nil {
		t.Fatal("expected error when reading path outside spec root")
	}
}

func TestFileSpecStorageProvider_ContextCancelled(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p, err := NewFileSpecStorageProvider(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = p.ReadSpecBytes(ctx, filepath.Join(dir, "any.yaml"))
	if err == nil {
		t.Fatal("expected cancellation error")
	}
}
