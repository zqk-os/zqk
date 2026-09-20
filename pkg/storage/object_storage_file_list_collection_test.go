package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestUsesBucketedStorage_RechecksWhenDirChanges(t *testing.T) {
	tmp := t.TempDir()
	kindDir := filepath.Join(tmp, "glossary_terms")
	if err := fileutil.EnsureDir(kindDir); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	f := &FileObjectStorage{}

	// Initial call caches "not bucketed" (no subdirs).
	if got := f.usesBucketedStorage("glossary_term", kindDir); got {
		t.Fatalf("expected false before subdirs exist")
	}

	// Ensure mtime changes reliably on fast filesystems by sleeping a tick.
	time.Sleep(2 * time.Millisecond)

	// Seed bucket list cache; it must be invalidated when kindDir changes.
	f.bucketListCache.Store(kindDir, []string{"old"})

	// Introduce a bucket subdir after the initial cache decision.
	if err := fileutil.EnsureDir(filepath.Join(kindDir, "a")); err != nil {
		t.Fatalf("mkdir bucket: %v", err)
	}

	// Should now return true (must not stick on cached false).
	if got := f.usesBucketedStorage("glossary_term", kindDir); !got {
		t.Fatalf("expected true after subdir exists")
	}

	if _, ok := f.bucketListCache.Load(kindDir); ok {
		t.Fatalf("expected bucketListCache invalidated on dir change")
	}
}

func TestCollectFilePaths_UnrecognizedDirectory(t *testing.T) {
	// A simple unit test that creates a FileObjectStorage directly without cli.NewProcessorForTests
	tmp := t.TempDir()
	f := &FileObjectStorage{
		processDir: tmp,
	}

	kindDir := filepath.Join(tmp, "unrecognized_dir")
	if err := fileutil.EnsureDir(kindDir); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Create a dummy yaml file
	if err := fileutil.WriteFile(filepath.Join(kindDir, "test.yaml"), []byte("foo: bar"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	// eventLogger may be nil (the collect path guards it). Context must not be nil (SA1012).
	paths, err := f.collectFilePaths(context.TODO(), kindDir, nil, nil)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(paths) != 0 {
		t.Fatalf("Expected empty paths for unrecognized directory, got: %v", paths)
	}
}
