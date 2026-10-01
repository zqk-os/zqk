package domain

import (
	"context"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/cliapp"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

// TestDomainDiscover_Integration runs domain discover with a test project root and asserts output.
// Not parallel: redirects os.Stdout; run sequentially with other integration tests in this package.
func TestDomainDiscover_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	testRoot, _ := setupIntegrationTestWithSpecsDomain(t, "domain-discover-integration")
	t.Cleanup(func() {
		_ = storagepkg.RunProjectTestTeardown(storagepkg.TempProjectTeardown(testRoot, nil))
	})

	cmd := NewDiscoverCmd()
	cmd.SetArgs([]string{})
	cli.SetContext(cmd, cli.ContextForProjectRoot(testRoot))

	var execErr error
	out := captureStdoutDomain(t, func() { execErr = cmd.Execute() })
	if execErr != nil {
		t.Fatalf("Execute domain discover: %v", execErr)
	}
	// Empty registry: expect "No domain registries" or "Discovered 0"
	if !strings.Contains(out, "domain registr") && !strings.Contains(out, "No domain registries") {
		t.Errorf("output should mention domain registries, got:\n%s", out)
	}
}

// TestDomainRegister_Integration runs domain register and asserts output and storage.
// Not parallel: redirects os.Stdout; run sequentially with other integration tests in this package.
// Uses test-owned storage so cleanup can shut it down and release file handles before t.TempDir() cleanup.
func TestDomainRegister_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	testRoot, _ := setupIntegrationTestWithSpecsDomain(t, "domain-register-integration")

	// Create storage we control so we can shut it down in cleanup (releases WAL and .zqk/process handles).
	storage, err := storagepkg.NewFileObjectStorageForTest(testRoot)
	if err != nil {
		t.Fatalf("create test storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, testRoot, storage)
	t.Cleanup(func() {
		if err := storagepkg.RunProjectTestTeardown(storagepkg.TempProjectTeardown(testRoot, storage)); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	cmd := NewRegisterCmd()
	cmd.SetArgs([]string{})
	_ = cmd.Flags().Set("domain-id", "integration-test-domain")
	_ = cmd.Flags().Set("namespace", "domain:integration:*")
	cli.SetContext(cmd, cli.ContextForProjectRoot(testRoot))
	// Inject storage so the command uses our instance and we can shut it down (avoids "directory not empty" on temp cleanup).
	reqCtx := cmd.Context()
	if reqCtx == nil {
		reqCtx = context.Background()
	}
	cmd.SetContext(cli.WithStorageProvider(reqCtx, storage))

	var execErr error
	out := captureStdoutDomain(t, func() { execErr = cmd.Execute() })
	if execErr != nil {
		t.Fatalf("Execute domain register: %v", execErr)
	}
	if !strings.Contains(out, "Domain registered") {
		t.Errorf("output should contain 'Domain registered', got:\n%s", out)
	}
	if !strings.Contains(out, "integration-test-domain") {
		t.Errorf("output should contain domain id 'integration-test-domain', got:\n%s", out)
	}
	if !strings.Contains(out, "DOMAIN-REG") {
		t.Errorf("output should contain registry id (e.g. DOMAIN-REG-001), got:\n%s", out)
	}
}
