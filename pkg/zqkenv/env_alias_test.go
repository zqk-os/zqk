package zqkenv_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestProjectRoot_BrandAliasFallback(t *testing.T) {
	// Simulate running as zcom binary
	brand.SetExecutableName("zcom")
	defer brand.SetExecutableName("zqk")

	tmpDir := t.TempDir()
	origZCOM := os.Getenv("ZCOM_PROJECT_ROOT")
	origZQK := zqkenv.ZQKProjectRoot().Get()
	origTestZCOM := os.Getenv("ZCOM_TEST_ROOT")
	origTestZQK := zqkenv.ZQKTestRoot().Get()
	defer func() {
		os.Setenv("ZCOM_PROJECT_ROOT", origZCOM)
		_ = zqkenv.ZQKProjectRoot().Set(origZQK)
		os.Setenv("ZCOM_TEST_ROOT", origTestZCOM)
		_ = zqkenv.ZQKTestRoot().Set(origTestZQK)
	}()

	os.Unsetenv("ZCOM_PROJECT_ROOT")
	_ = zqkenv.ZQKProjectRoot().Set(tmpDir)

	// zqkenv.ProjectRoot().Get() should return tmpDir via ZQK_PROJECT_ROOT fallback
	got := zqkenv.ProjectRoot().Get()
	if got != tmpDir {
		t.Fatalf("expected zqkenv.ProjectRoot().Get() to return %q, got %q", tmpDir, got)
	}

	// OrDefault should also use fallback
	if od := zqkenv.ProjectRoot().OrDefault("fallback"); od != tmpDir {
		t.Fatalf("expected OrDefault to return %q, got %q", tmpDir, od)
	}

	// ResolveProjectRoot should resolve to tmpDir
	resolved := paths.ResolveProjectRoot(tmpDir)
	expectedClean, _ := filepath.Abs(tmpDir)
	if resolved != expectedClean {
		t.Fatalf("expected ResolveProjectRoot to resolve to %q, got %q", expectedClean, resolved)
	}

	// Branded env takes precedence over fallback when both are set
	brandedDir := t.TempDir()
	os.Setenv("ZCOM_PROJECT_ROOT", brandedDir)
	if gotBranded := zqkenv.ProjectRoot().Get(); gotBranded != brandedDir {
		t.Fatalf("expected branded env %q to take precedence, got %q", brandedDir, gotBranded)
	}

	// TestRoot fallback
	testDir := t.TempDir()
	os.Unsetenv("ZCOM_TEST_ROOT")
	_ = zqkenv.ZQKTestRoot().Set(testDir)
	if gotTest := zqkenv.TestRoot().Get(); gotTest != testDir {
		t.Fatalf("expected TestRoot() fallback to return %q, got %q", testDir, gotTest)
	}
}
