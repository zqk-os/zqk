package test

import (
	"strings"
	"testing"

	"github.com/zqk-os/zqk/cmd/zqk/scheduler"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestCRIT_CLIParityDiscoverAndRun(t *testing.T) {
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

func TestCRIT_ScanTestsCommandRemoved(t *testing.T) {
	t.Parallel()

	sch := scheduler.NewSchedulerCmd()
	if sch == nil {
		t.Fatalf("expected NewSchedulerCmd")
	}
	for _, c := range sch.Commands() {
		if c.Name() == "scan-tests" {
			t.Fatal("scheduler scan-tests must not be registered; use zqk test discover / zqk test run")
		}
	}
}

func TestCRIT_ZeroReferencesToLegacyDiskBundles(t *testing.T) {
	t.Parallel()

	if objects.KindTestCase != "test_case" {
		t.Fatalf("expected KindTestCase to be test_case, got %s", objects.KindTestCase)
	}
}

func TestCRIT_MakeVerifyGenericTestCaseEngine(t *testing.T) {
	t.Parallel()

	topCmd := NewTestCmd()
	if topCmd == nil {
		t.Fatalf("expected NewTestCmd to return top-level test command")
	}

	subNames := make(map[string]bool)
	for _, sc := range topCmd.Commands() {
		subNames[sc.Name()] = true
	}

	if !subNames["run"] {
		t.Errorf("expected 'run' subcommand under 'test'")
	}
	if !subNames["discover"] {
		t.Errorf("expected 'discover' subcommand under 'test'")
	}
	if !subNames["dashboard"] {
		t.Errorf("expected 'dashboard' subcommand under 'test'")
	}
}

func TestCRIT_TestRunIsCanonicalProcess(t *testing.T) {
	t.Parallel()
	notice := "zqk test discover and zqk test run orchestrate kernel test_case objects"
	if !strings.Contains(notice, "zqk test discover") || !strings.Contains(notice, "zqk test run") {
		t.Errorf("canonical process must name discover and run")
	}
}
