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
	cmd.SetArgs([]string{"--domain-id", "integration-test-domain", "--namespace", "domain:integration:*"})
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

func TestDomainCmd_Hierarchy(t *testing.T) {
	cmd := NewDomainCmd()
	if cmd == nil {
		t.Fatal("expected non-nil domain command")
	}
	if cmd.Use != "domain" {
		t.Errorf("expected use 'domain', got %q", cmd.Use)
	}
	subNames := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subNames[sub.Name()] = true
	}
	if !subNames["discover"] {
		t.Error("expected 'discover' subcommand under domain")
	}
	if !subNames["register"] {
		t.Error("expected 'register' subcommand under domain")
	}
}

func TestDomainRegister_ValidationAndDryRun(t *testing.T) {
	testRoot, _ := setupIntegrationTestWithSpecsDomain(t, "domain-val-dryrun")
	t.Cleanup(func() {
		_ = storagepkg.RunProjectTestTeardown(storagepkg.TempProjectTeardown(testRoot, nil))
	})

	// Missing --domain-id should fail
	cmdFail := NewRegisterCmd()
	cmdFail.SetArgs([]string{"--namespace", "domain:test:*"})
	cli.SetContext(cmdFail, cli.ContextForProjectRoot(testRoot))
	err := cmdFail.Execute()
	if err == nil {
		t.Fatal("expected error when --domain-id is missing")
	}
	if !strings.Contains(err.Error(), "--domain-id is required") {
		t.Errorf("unexpected error message: %v", err)
	}

	// Dry run should succeed and print preview
	cmdDry := NewRegisterCmd()
	cmdDry.SetArgs([]string{"--domain-id", "dry-domain", "--namespace", "domain:dry:*", "--dry-run"})
	cli.SetContext(cmdDry, cli.ContextForProjectRoot(testRoot))
	var dryErr error
	dryOut := captureStdoutDomain(t, func() { dryErr = cmdDry.Execute() })
	if dryErr != nil {
		t.Fatalf("dry run failed: %v", dryErr)
	}
	if !strings.Contains(dryOut, "Dry run: would register domain") {
		t.Errorf("expected dry run preview, got:\n%s", dryOut)
	}
}

func TestDomainRegister_UpdateExistingAndDiscover(t *testing.T) {
	testRoot, _ := setupIntegrationTestWithSpecsDomain(t, "domain-update-discover")

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

	// 1. Register first domain
	cmd1 := NewRegisterCmd()
	cmd1.SetArgs([]string{"--domain-id", "dom-alpha", "--namespace", "domain:alpha:*"})
	cli.SetContext(cmd1, cli.ContextForProjectRoot(testRoot))
	cmd1.SetContext(cli.WithStorageProvider(context.Background(), storage))
	var execErr1 error
	out1 := captureStdoutDomain(t, func() { execErr1 = cmd1.Execute() })
	if execErr1 != nil {
		t.Fatalf("first domain register failed: %v", execErr1)
	}
	if !strings.Contains(out1, "Domain registered: dom-alpha") {
		t.Errorf("expected dom-alpha registration output, got: %s", out1)
	}

	// 2. Register second domain into existing registry (exercises update branch)
	cmd2 := NewRegisterCmd()
	cmd2.SetArgs([]string{"--domain-id", "dom-beta", "--namespace", "domain:beta:*", "--spec-file", "specs/beta.yaml"})
	cli.SetContext(cmd2, cli.ContextForProjectRoot(testRoot))
	cmd2.SetContext(cli.WithStorageProvider(context.Background(), storage))
	var execErr2 error
	out2 := captureStdoutDomain(t, func() { execErr2 = cmd2.Execute() })
	if execErr2 != nil {
		t.Fatalf("second domain register failed: %v", execErr2)
	}
	if !strings.Contains(out2, "Domain registered: dom-beta") {
		t.Errorf("expected dom-beta registration output, got: %s", out2)
	}

	// 3. Discover populated registries
	discCmd := NewDiscoverCmd()
	discCmd.SetArgs([]string{})
	cli.SetContext(discCmd, cli.ContextForProjectRoot(testRoot))
	discCmd.SetContext(cli.WithStorageProvider(context.Background(), storage))
	var discErr error
	discOut := captureStdoutDomain(t, func() { discErr = discCmd.Execute() })
	if discErr != nil {
		t.Fatalf("domain discover failed: %v", discErr)
	}
	if !strings.Contains(discOut, "Discovered") {
		t.Errorf("expected 'Discovered' in output, got: %s", discOut)
	}
	if !strings.Contains(discOut, "DOMAIN-REG") {
		t.Errorf("expected 'DOMAIN-REG' in output, got: %s", discOut)
	}
}
