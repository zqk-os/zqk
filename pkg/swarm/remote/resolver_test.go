package remote

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type mockCloner struct {
	clonedURL string
	destDir   string
	err       error
	writeSpec bool
}

func (m *mockCloner) Clone(ctx context.Context, cloneURL, destDir string) error {
	m.clonedURL = cloneURL
	m.destDir = destDir
	if m.err != nil {
		return m.err
	}
	if m.writeSpec {
		if err := fileutil.MkdirAll(destDir, paths.DirPerm755); err != nil {
			return err
		}
		manifestContent := []byte("schema_version: 1.0.0\nname: remote-swarm\nversion: 0.1.0\ndescription: Test swarm\n")
		return fileutil.WriteFile(filepath.Join(destDir, "swarm.yaml"), manifestContent, paths.FilePerm644)
	}
	return nil
}

func TestIsRemoteTarget(t *testing.T) {
	tests := []struct {
		target   string
		expected bool
	}{
		{"", false},
		{".", false},
		{"./swarm.yaml", false},
		{"/tmp/swarm.yaml", false},
		{"~/swarms/foo", false},
		{"github.com/zqk-os/refactor-swarm", true},
		{"gitlab.com/org/project", true},
		{"https://github.com/zqk-os/refactor-swarm.git", true},
		{"git@github.com:zqk-os/refactor-swarm.git", true},
	}

	for _, tt := range tests {
		if got := IsRemoteTarget(tt.target); got != tt.expected {
			t.Errorf("IsRemoteTarget(%q) = %v; want %v", tt.target, got, tt.expected)
		}
	}
}

func TestNormalizeCloneURL(t *testing.T) {
	tests := []struct {
		target      string
		expectedURL string
		expectSlug  string
	}{
		{"github.com/foo/bar", "https://github.com/foo/bar.git", "foo/bar"},
		{"https://github.com/foo/bar.git", "https://github.com/foo/bar.git", "foo/bar"},
	}

	for _, tt := range tests {
		url, slug, err := NormalizeCloneURL(tt.target)
		if err != nil {
			t.Fatalf("NormalizeCloneURL(%q) unexpected error: %v", tt.target, err)
		}
		if url != tt.expectedURL {
			t.Errorf("NormalizeCloneURL(%q) URL = %q; want %q", tt.target, url, tt.expectedURL)
		}
		if slug != tt.expectSlug {
			t.Errorf("NormalizeCloneURL(%q) Slug = %q; want %q", tt.target, slug, tt.expectSlug)
		}
	}
}

func TestResolveRemoteSwarm(t *testing.T) {
	tempCache := t.TempDir()

	mock := &mockCloner{writeSpec: true}
	ctx := context.Background()

	opts := ResolveOptions{
		CacheDir: tempCache,
		Cloner:   mock,
		Timeout:  5 * time.Second,
	}

	manifestPath, err := Resolve(ctx, "github.com/example/my-swarm", opts)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if mock.clonedURL != "https://github.com/example/my-swarm.git" {
		t.Errorf("Expected clone URL %q, got %q", "https://github.com/example/my-swarm.git", mock.clonedURL)
	}

	expectedManifest := filepath.Join(tempCache, "example", "my-swarm", "swarm.yaml")
	if manifestPath != expectedManifest {
		t.Errorf("Expected manifest %s, got %s", expectedManifest, manifestPath)
	}

	// Verify caching on second resolve (should not call clone again)
	mock.clonedURL = ""
	cachedPath, err := Resolve(ctx, "github.com/example/my-swarm", opts)
	if err != nil {
		t.Fatalf("Resolve cached failed: %v", err)
	}
	if cachedPath != manifestPath {
		t.Errorf("Expected cached path %s, got %s", manifestPath, cachedPath)
	}
	if mock.clonedURL != "" {
		t.Errorf("Expected cache hit, but clone was called: %s", mock.clonedURL)
	}
}
