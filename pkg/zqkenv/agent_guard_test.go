package zqkenv

import (
	"strings"
	"testing"
)

func TestForegroundGoTestAllowed_MakeFlags(t *testing.T) {
	t.Setenv(JobID(), "")
	t.Setenv(TestRoot(), "")

	t.Setenv(InTest(), "")
	t.Setenv(UpdateHelpGolden(), "")
	t.Setenv("ZQKCLI_TEST_UPDATE_HELP_GOLDEN", "")
	t.Setenv("CI", "")
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("GITLAB_CI", "")
	t.Setenv("MAKELEVEL", "")
	t.Setenv("MAKEFLAGS", "w")
	if !foregroundGoTestAllowed() {
		t.Fatal("expected MAKEFLAGS to allow foreground go test")
	}
}

func TestForegroundGoTestAllowed_JobID(t *testing.T) {
	t.Setenv("MAKEFLAGS", "")
	t.Setenv("MAKELEVEL", "")
	t.Setenv(TestRoot(), "")
	t.Setenv(JobID(), "SCH-test")
	if !foregroundGoTestAllowed() {
		t.Fatal("expected JobID to allow foreground go test")
	}
}

// clearForegroundGoTestEnv blanks every arm of foregroundGoTestAllowed so a test can assert that
// one specific arm is what granted permission, rather than inheriting a grant from the ambient
// environment (which, in this repo, usually has several of these set).
func clearForegroundGoTestEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		JobID(), "ZQK_JOB_ID", "ZQK_SCHEDULER_JOB_ID",
		AllowForegroundGoTest(), "ZQK_ALLOW_FOREGROUND_GO_TEST",
		TestRoot(), "ZQK_TEST_ROOT",
		InTest(), UpdateHelpGolden(), "ZQKCLI_TEST_UPDATE_HELP_GOLDEN",
		"MAKEFLAGS", "MAKELEVEL", "CI", "GITHUB_ACTIONS", "GITLAB_CI",
	} {
		if k != "" {
			t.Setenv(k, "")
		}
	}
}

func TestForegroundGoTestAllowed_AllowForegroundGoTest(t *testing.T) {
	clearForegroundGoTestEnv(t)
	if foregroundGoTestAllowed() {
		t.Fatal("fixture invalid: permission granted with every arm cleared, so this test proves nothing")
	}
	t.Setenv(AllowForegroundGoTest(), "1")
	if !foregroundGoTestAllowed() {
		t.Fatalf("expected %s=1 to allow foreground go test", AllowForegroundGoTest())
	}
}

// TestForegroundGoTestPanicRecommendsAPermissionNotAPath is the whole point of the split.
//
// The guard used to tell operators to bypass it by setting TEST_ROOT. That variable also relocates
// the CLI's project root, so obeying the message broke every subsequent zqk command in that shell
// with an error ("failed to load account schema") that pointed at neither the guard nor the
// variable. A refusal message is an instruction; instructing a side effect the reader cannot see is
// the defect, and this test refuses to let the advice regress to a path variable.
func TestForegroundGoTestPanicRecommendsAPermissionNotAPath(t *testing.T) {
	msg := foregroundGoTestPanicMessage()

	permission := AllowForegroundGoTest()
	if !strings.Contains(msg, permission) {
		t.Errorf("bypass advice must name %s, the variable that grants only permission; got:\n%s", permission, msg)
	}

	// TestRoot may appear, but only as a warning. Assert it is never offered as the way to bypass.
	root := TestRoot()
	for _, line := range strings.Split(msg, "\n") {
		if !strings.Contains(line, root) {
			continue
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "bypass") || strings.Contains(lower, "or set "+strings.ToLower(root)) {
			t.Errorf("bypass advice names the path variable %s as a way through the guard; setting it "+
				"relocates the project root and breaks later commands in the same shell. Line:\n  %s", root, line)
		}
		if !strings.Contains(lower, "not") {
			t.Errorf("%s is mentioned without warning against it; a reader will set it. Line:\n  %s", root, line)
		}
	}
}

func TestIsGoTestBinary(t *testing.T) {
	if !isGoTestBinary("/tmp/zqkenv.test") {
		t.Fatal("expected .test suffix")
	}
	if isGoTestBinary("/tmp/zqk") {
		t.Fatal("plain binary should not match")
	}
}
