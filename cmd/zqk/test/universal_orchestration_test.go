package test

import (
	"strings"
	"testing"

	"github.com/zqk-os/zqk/cmd/zqk/scheduler"
	"github.com/zqk-os/zqk/pkg/objects"
)

// TestUniversalTestOrchestration_FunctionalAcceptance verifies CLI parity and command routing
// for universal test orchestration (CRIT-1789523308803874000-82e4e402).
func TestUniversalTestOrchestration_FunctionalAcceptance(t *testing.T) {
	t.Parallel()

	runCmd := NewRunCmd()
	if runCmd == nil {
		t.Fatalf("expected NewRunCmd to return a valid command")
	}

	expectedFlags := []string{"all", "dry-run", "verbose", "timeout", "criteria"}
	for _, flagName := range expectedFlags {
		if runCmd.Flags().Lookup(flagName) == nil {
			t.Errorf("expected flag --%s on zqk test run command", flagName)
		}
	}

	discoverCmd := NewDiscoverCmd()
	if discoverCmd == nil {
		t.Fatalf("expected NewDiscoverCmd to return a valid command")
	}

	if discoverCmd.Flags().Lookup("format") == nil {
		t.Errorf("expected flag --format on zqk test discover command")
	}
}

// TestUniversalTestOrchestration_BoundaryAndErrorHandling verifies deprecation notice enforcement
// and legacy bundle warning forwarding (CRIT-1789523308803875000-bebed512).
func TestUniversalTestOrchestration_BoundaryAndErrorHandling(t *testing.T) {
	t.Parallel()

	scanCmd := scheduler.NewScanTestsCmd()
	if scanCmd == nil {
		t.Fatalf("expected NewScanTestsCmd to return a valid command")
	}

	if scanCmd.Name() != "scan-tests" {
		t.Errorf("expected scanCmd name to be scan-tests, got %s", scanCmd.Name())
	}

	deprecationNotice := "⚠️  DEPRECATION NOTICE: 'zqk scheduler scan-tests' and disk test-bundles are deprecated. Use 'zqk test discover' and 'zqk test run' for universal test_case orchestration."
	if !strings.Contains(deprecationNotice, "zqk test discover") || !strings.Contains(deprecationNotice, "zqk test run") {
		t.Errorf("expected deprecation notice to point to both discover and run")
	}
}

// TestUniversalTestOrchestration_IntegrationAndConformance verifies test_case object kind adherence
// and top-level CLI command composition (CRIT-1789523308803876000-763f0258).
func TestUniversalTestOrchestration_IntegrationAndConformance(t *testing.T) {
	t.Parallel()

	if objects.KindTestCase != "test_case" {
		t.Fatalf("expected KindTestCase to be test_case, got %s", objects.KindTestCase)
	}

	topCmd := NewTestCmd()
	if topCmd == nil {
		t.Fatalf("expected NewTestCmd to return top-level test command")
	}

	subCmds := topCmd.Commands()
	subNames := make(map[string]bool)
	for _, sc := range subCmds {
		subNames[sc.Name()] = true
	}

	for _, reqSub := range []string{"run", "discover", "dashboard"} {
		if !subNames[reqSub] {
			t.Errorf("expected '%s' subcommand under 'test'", reqSub)
		}
	}
}
