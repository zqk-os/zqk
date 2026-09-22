package zqkenv

import (
	"strings"
	"testing"
)

func TestForegroundGoTestAllowed_MakeFlags(t *testing.T) {
	t.Setenv(JobID().Name(), "")
	t.Setenv(TestRoot().Name(), "")

	t.Setenv(InTest().Name(), "")
	t.Setenv(UpdateHelpGolden().Name(), "")
	t.Setenv(ZQKCLITestUpdateHelpGolden().Name(), "")
	t.Setenv(EnvCI().Name(), "")
	t.Setenv(GithubActions().Name(), "")
	t.Setenv(GitlabCI().Name(), "")
	t.Setenv(MakeLevel().Name(), "")
	t.Setenv(MakeFlags().Name(), "w")
	if !foregroundGoTestAllowed() {
		t.Fatal("expected MAKEFLAGS to allow foreground go test")
	}
}

func TestForegroundGoTestAllowed_JobID(t *testing.T) {
	t.Setenv(MakeFlags().Name(), "")
	t.Setenv(MakeLevel().Name(), "")
	t.Setenv(TestRoot().Name(), "")
	t.Setenv(JobID().Name(), "SCH-test")
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
		JobID().Name(), "ZQK_JOB_ID", "ZQK_SCHEDULER_JOB_ID",
		AllowForegroundGoTest().Name(), "ZQK_ALLOW_FOREGROUND_GO_TEST",
		TestRoot().Name(), "ZQK_TEST_ROOT",
		InTest().Name(), UpdateHelpGolden().Name(), "ZQKCLI_TEST_UPDATE_HELP_GOLDEN",
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
	t.Setenv(AllowForegroundGoTest().Name(), "1")
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
	if !strings.Contains(msg, permission.Name()) {
		t.Errorf("bypass advice must name %s, the variable that grants only permission; got:\n%s", permission, msg)
	}

	// TestRoot may appear, but only as a warning. Assert it is never offered as the way to bypass.
	root := TestRoot().Name()
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
	if !isGoTestBinary("/tmp/foo.test") {
		t.Fatal("expected .test suffix")
	}
	if !IsGoTestBinary("/tmp/foo.test") {
		t.Fatal("exported IsGoTestBinary must match")
	}
	if isGoTestBinary("/tmp/zqk") {
		t.Fatal("plain binary should not match")
	}
}
