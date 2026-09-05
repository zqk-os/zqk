package paths

import (
	"path/filepath"
	"testing"
)

func TestPathRefFromRelPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		rel  string
		want string
	}{
		{"docs/foo.md", "prefix:docs/foo.md"},
		{"docs/architecture/PATH_ALIAS_RESOLUTION.md", "prefix:docs/architecture/PATH_ALIAS_RESOLUTION.md"},
		{"", ""},
	}
	for _, tt := range tests {
		got := PathRefFromRelPath(tt.rel)
		if got != tt.want {
			t.Errorf("PathRefFromRelPath(%q) = %q, want %q", tt.rel, got, tt.want)
		}
	}
}

func TestNormalizeDocEntryPathForKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		pathRef string
		want    string
	}{
		{"prefix:docs/foo.md", "docs/foo.md"},
		{"docs/foo.md", "docs/foo.md"},
		{"prefix:docs/architecture/bar.md", "docs/architecture/bar.md"},
		{"", ""},
	}
	for _, tt := range tests {
		got := NormalizeDocEntryPathForKey(tt.pathRef)
		if got != tt.want {
			t.Errorf("NormalizeDocEntryPathForKey(%q) = %q, want %q", tt.pathRef, got, tt.want)
		}
	}
}

func TestResolveDocEntryPath_LegacyRelative(t *testing.T) {
	t.Parallel()
	root := filepath.Clean("/project/root")
	got, err := ResolveDocEntryPath(root, "docs/foo.md")
	if err != nil {
		t.Fatalf("ResolveDocEntryPath: %v", err)
	}
	want := filepath.Join(root, "docs/foo.md")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveDocEntryPath_PrefixRequiresCache(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// No cache built; prefix: alias should fail
	_, err := ResolveDocEntryPath(root, "prefix:docs/foo.md")
	if err == nil {
		t.Fatal("expected error when prefix: alias not in cache")
	}
	if err != ErrPathAliasNotInCache {
		t.Errorf("expected ErrPathAliasNotInCache, got %v", err)
	}
}

func TestResolveDocEntryPath_Empty(t *testing.T) {
	t.Parallel()
	got, err := ResolveDocEntryPath("/root", "")
	if err != nil {
		t.Fatalf("ResolveDocEntryPath: %v", err)
	}
	if got != emptyValue {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestReplacePathCache_and_AddPathAlias(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// Replace with initial map
	ReplacePathCache(root, map[string]string{"docs": "docs", "process": "docs/process"})
	if !IsPathCacheBuilt(root) {
		t.Fatal("cache should be built after ReplacePathCache")
	}
	if GetPathAlias(root, "docs") != "docs" {
		t.Errorf("expected docs alias")
	}
	// Incremental add
	AddPathAlias(root, "foo", "custom/foo")
	if GetPathAlias(root, "foo") != "custom/foo" {
		t.Errorf("expected foo alias after AddPathAlias")
	}
	// Remove
	RemovePathAlias(root, "foo")
	if GetPathAlias(root, "foo") != emptyValue {
		t.Errorf("expected foo removed")
	}
	// Resolve still works for existing
	abs, err := ResolvePathStrict(root, "prefix:docs")
	want := filepath.Join(root, "docs")
	if err != nil || abs != want {
		t.Errorf("ResolvePathStrict(prefix:docs) = %q, %v; want %q", abs, err, want)
	}
}

// TestResolvePathStrict_processInternalSpecIndexSuffix ensures prefix:process_internal/spec_index.json
// resolves when the cache only has the process_internal base dir (see TryLoadSpecIndexForProjectRoot).
func TestResolvePathStrict_processInternalSpecIndexSuffix(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	ReplacePathCache(root, DefaultPathAliases())
	ref := PathSchemePrefix + "process_internal/spec_index.json"
	abs, err := ResolvePathStrict(root, ref)
	if err != nil {
		t.Fatalf("ResolvePathStrict(%q): %v", ref, err)
	}
	want := filepath.Join(root, ProcessInternalDir, "spec_index.json")
	if abs != want {
		t.Errorf("got %q, want %q", abs, want)
	}
}
