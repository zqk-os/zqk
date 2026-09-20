package testdiscovery

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDiscoveryCache_GetSetSave(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "cache.json")

	cache := LoadCache(cachePath)
	if cache == nil {
		t.Fatal("expected non-nil cache")
	}

	targets := []DiscoveredTarget{
		{
			Language: "go",
			Path:     "pkg/sample/sample_test.go",
			Function: "TestSample",
			Line:     10,
		},
	}

	now := time.Now().Truncate(time.Second)
	content := []byte("package sample\nfunc TestSample(t *testing.T) {}\n")

	// Initially empty
	if _, ok := cache.Get("pkg/sample/sample_test.go", now, content); ok {
		t.Fatal("expected cache miss on empty cache")
	}

	// Set entry
	cache.Set("pkg/sample/sample_test.go", now, content, targets)

	// Get entry by mtime
	got, ok := cache.Get("pkg/sample/sample_test.go", now, nil)
	if !ok || len(got) != 1 || got[0].Function != "TestSample" {
		t.Fatalf("expected cached targets, got %v (ok=%v)", got, ok)
	}

	// Get entry by content hash fallback (different mtime)
	got2, ok2 := cache.Get("pkg/sample/sample_test.go", now.Add(time.Hour), content)
	if !ok2 || len(got2) != 1 {
		t.Fatalf("expected hash fallback hit, got %v (ok=%v)", got2, ok2)
	}

	// Save cache
	if err := cache.Save(); err != nil {
		t.Fatalf("failed to save cache: %v", err)
	}

	// Reload cache
	reloaded := LoadCache(cachePath)
	gotReloaded, okReloaded := reloaded.Get("pkg/sample/sample_test.go", now, nil)
	if !okReloaded || len(gotReloaded) != 1 {
		t.Fatalf("expected reloaded cache to have entry, got %v (ok=%v)", gotReloaded, okReloaded)
	}
}
