package zqkenv_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// TestBrandEnvParity_FunctionalAcceptance validates CRIT-1789651024548820000-f305050d:
// Pure Go brand environment variable resolution handles ZQK_* and custom brand prefixes.
func TestBrandEnvParity_FunctionalAcceptance(t *testing.T) {
	// Preserve state and restore upon test completion
	origExecutable := brand.ExecutableName()
	defer brand.SetExecutableName(origExecutable)

	// Test case 1: Default brand (ZQK)
	brand.SetExecutableName("zqk")
	if pfx := brand.EnvPrefix(); pfx != "ZQK" {
		t.Fatalf("expected EnvPrefix to be ZQK, got %q", pfx)
	}

	testVal := "/tmp/test-zqk-root"
	t.Setenv(zqkenv.DefaultBrandKey("PROJECT_ROOT"), testVal)
	if got := zqkenv.ProjectRoot().Get(); got != testVal {
		t.Fatalf("expected zqkenv.ProjectRoot().Get() = %q, got %q", testVal, got)
	}
	if od := zqkenv.ProjectRoot().OrDefault("/fallback"); od != testVal {
		t.Fatalf("expected OrDefault = %q, got %q", testVal, od)
	}

	// Test case 2: Community Edition brand (ZCOM)
	brand.SetExecutableName("zcom")
	if pfx := brand.EnvPrefix(); pfx != "ZCOM" {
		t.Fatalf("expected EnvPrefix to be ZCOM, got %q", pfx)
	}
	if name := zqkenv.ProjectRoot().Name(); name != "ZCOM_PROJECT_ROOT" {
		t.Fatalf("expected ProjectRoot name to be ZCOM_PROJECT_ROOT, got %q", name)
	}

	// Fallback to ZQK_* when ZCOM_* is unset
	t.Setenv("ZCOM_PROJECT_ROOT", "")
	t.Setenv(zqkenv.DefaultBrandKey("PROJECT_ROOT"), testVal)
	if got := zqkenv.ProjectRoot().Get(); got != testVal {
		t.Fatalf("expected fallback to ZQK_PROJECT_ROOT (%q), got %q", testVal, got)
	}

	// Precedence: ZCOM_* overrides ZQK_* when set
	zcomVal := "/tmp/test-zcom-root"
	t.Setenv("ZCOM_PROJECT_ROOT", zcomVal)
	if got := zqkenv.ProjectRoot().Get(); got != zcomVal {
		t.Fatalf("expected ZCOM_PROJECT_ROOT (%q) to take precedence over ZQK_PROJECT_ROOT, got %q", zcomVal, got)
	}

	// Test case 3: Arbitrary custom brand prefix (e.g. ACME_TOOL)
	brand.SetExecutableName("acme-tool")
	if pfx := brand.EnvPrefix(); pfx != "ACME_TOOL" {
		t.Fatalf("expected EnvPrefix to be ACME_TOOL, got %q", pfx)
	}
	acmeVal := "/tmp/test-acme-root"
	t.Setenv("ACME_TOOL_PROJECT_ROOT", acmeVal)
	if got := zqkenv.ProjectRoot().Get(); got != acmeVal {
		t.Fatalf("expected ACME_TOOL_PROJECT_ROOT = %q, got %q", acmeVal, got)
	}
}

// TestBrandEnvParity_BoundaryAndErrorHandling validates CRIT-1789651024548821000-42a9a8b5:
// Boundary condition validation, negative testing, invalid input rejection, and failure recovery.
func TestBrandEnvParity_BoundaryAndErrorHandling(t *testing.T) {
	origExecutable := brand.ExecutableName()
	defer brand.SetExecutableName(origExecutable)

	// Boundary 1: Channel suffixes stripped cleanly
	for _, tc := range []struct {
		input    string
		expected string
	}{
		{"zqk-stable", "ZQK"},
		{"zqk-community", "ZQK"},
		{"zcom-stable", "ZCOM"},
		{"zcom-beta", "ZCOM"},
		{"acme-dev", "ACME"},
		{"tool-mcp-daemon", "TOOL"},
	} {
		brand.SetExecutableName(tc.input)
		if pfx := brand.EnvPrefix(); pfx != tc.expected {
			t.Errorf("stripEnvChannelSuffix(%q) = %q, want %q", tc.input, pfx, tc.expected)
		}
	}

	// Boundary 2: Required() panics when unset across both branded and fallback keys
	brand.SetExecutableName("zcom")
	t.Setenv("ZCOM_TEST_KEY_PANIC", "")
	t.Setenv(zqkenv.DefaultBrandKey("TEST_KEY_PANIC"), "")
	ev := zqkenv.EnvVar{Key: "ZCOM_TEST_KEY_PANIC"}

	func() {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatalf("expected Required() to panic on missing environment variable, but did not")
			}
			msg, ok := r.(string)
			if !ok || !strings.Contains(msg, "Missing required environment variable") {
				t.Fatalf("unexpected panic message: %v", r)
			}
		}()
		_ = ev.Required()
	}()

	// Boundary 3: Fail-closed TestRoot and ProjectRoot for unseated agent worktree paths
	unseatedWorktree := filepath.Join(fileutil.TempDir(), "zqk-worktrees", "test-repo", "ATK-failclosed-boundary")
	if err := fileutil.EnsureDir(filepath.Join(unseatedWorktree, paths.ProjectDataDir)); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fileutil.RemoveAll(unseatedWorktree) }()

	// Under ProjectRoot
	t.Setenv(zqkenv.DefaultBrandKey("PROJECT_ROOT"), unseatedWorktree)
	t.Setenv(zqkenv.DefaultBrandKey("TEST_ROOT"), "")
	if resolved := paths.ResolveProjectRoot(unseatedWorktree); resolved == unseatedWorktree {
		t.Fatalf("ResolveProjectRoot should refuse unseated agent worktree under ProjectRoot; got %q", resolved)
	}

	// Under TestRoot
	t.Setenv(zqkenv.DefaultBrandKey("PROJECT_ROOT"), "")
	t.Setenv(zqkenv.DefaultBrandKey("TEST_ROOT"), unseatedWorktree)
	if resolved := paths.ResolveProjectRoot(unseatedWorktree); resolved == unseatedWorktree {
		t.Fatalf("ResolveProjectRoot should refuse unseated agent worktree under TestRoot; got %q", resolved)
	}
}

// TestBrandEnvParity_IntegrationAndConformance validates CRIT-1789651024548822000-f0f04b16:
// System integration, contract conformance, observability, and regression verification.
func TestBrandEnvParity_IntegrationAndConformance(t *testing.T) {
	origExecutable := brand.ExecutableName()
	defer brand.SetExecutableName(origExecutable)

	// Integration 1: SubprocessEnvironWithTestRoot strips sensitive session tokens across brands
	brand.SetExecutableName("zcom")
	t.Setenv(zqkenv.DefaultBrandKey("SESSION"), "secret-zqk-session")
	t.Setenv("ZCOM_SESSION", "secret-zcom-session")
	t.Setenv("ZCOM_TEST_SESSION", "secret-zcom-test-session")

	subEnv := zqkenv.SubprocessEnvironWithTestRoot(t.TempDir())
	for _, entry := range subEnv {
		if strings.HasPrefix(entry, zqkenv.DefaultBrandKey("SESSION")+"=") ||
			strings.HasPrefix(entry, "ZCOM_SESSION=") ||
			strings.HasPrefix(entry, "ZCOM_TEST_SESSION=") {
			t.Fatalf("SubprocessEnvironWithTestRoot leaked sensitive session variable: %q", entry)
		}
	}

	// Integration 2: MaskSensitiveValue and SanitizeEnvironment conformance
	testEnvs := []string{
		zqkenv.DefaultBrandKey("TOKEN") + "=supersecret123",
		"ZCOM_API_KEY=apikey987",
		"APP_PUBLIC_VAR=public_value",
		"AUTHORIZATION=Bearer tokenXYZ",
		"CUSTOM_HEADER=Bearer tokenXYZ",
	}
	sanitized := zqkenv.SanitizeEnvironment(testEnvs)
	for _, kv := range sanitized {
		k, v, _ := strings.Cut(kv, "=")
		if k == zqkenv.DefaultBrandKey("TOKEN") || k == "ZCOM_API_KEY" || k == "AUTHORIZATION" {
			if v != "******" {
				t.Fatalf("expected %s to be masked with '******', got %q", k, v)
			}
		}
		if k == "CUSTOM_HEADER" {
			if v != "Bearer ******" {
				t.Fatalf("expected %s to be masked with 'Bearer ******', got %q", k, v)
			}
		}
		if k == "APP_PUBLIC_VAR" {
			if v != "public_value" {
				t.Fatalf("expected %s to remain unmasked, got %q", k, v)
			}
		}
	}

	// Integration 3: paths.ResolveProjectRoot dynamically adapts to brand switches
	testDir := t.TempDir()
	expectedAbs, _ := filepath.Abs(testDir)

	brand.SetExecutableName("zcom")
	t.Setenv("ZCOM_PROJECT_ROOT", testDir)
	t.Setenv(zqkenv.DefaultBrandKey("PROJECT_ROOT"), "")
	if got := paths.ResolveProjectRoot(testDir); got != expectedAbs {
		t.Fatalf("expected ResolveProjectRoot for zcom to be %q, got %q", expectedAbs, got)
	}

	brand.SetExecutableName("zqk")
	t.Setenv("ZCOM_PROJECT_ROOT", "")
	t.Setenv(zqkenv.DefaultBrandKey("PROJECT_ROOT"), testDir)
	if got := paths.ResolveProjectRoot(testDir); got != expectedAbs {
		t.Fatalf("expected ResolveProjectRoot for zqk to be %q, got %q", expectedAbs, got)
	}
}
