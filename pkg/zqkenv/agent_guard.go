package zqkenv

import (
	"os"
	"strings"
)

func init() {
	// Agent-facing poison pill: IDE/agent terminals must not run unbounded
	// foreground `go test` as a substitute for scheduler scan-tests.
	//
	// This must NEVER break Make, CI, or scripted verify steps that legitimately
	// invoke `go test` (e.g. make zqk-community → verify-bootstrap-portable.sh).
	arg0 := os.Args[0]
	if !isGoTestBinary(arg0) {
		return
	}
	if foregroundGoTestAllowed() {
		return
	}
	// TRACK: [Agent safety guard violation]
	panic(foregroundGoTestPanicMessage())
}

// foregroundGoTestPanicMessage is the guard's refusal text.
//
// Extracted from the panic so a test can read it. The bypass this advertises must grant a permission
// and nothing else: it previously named TestRoot(), which also relocates the CLI's project root, so
// an operator who followed this very message poisoned their shell — every later zqk command resolved
// against the temp directory and failed with "unauthorized: failed to load account schema", naming
// neither this guard nor the variable that caused it. Guarded by
// TestForegroundGoTestPanicRecommendsAPermissionNotAPath.
func foregroundGoTestPanicMessage() string {
	return "⚡️ BZZZT! AGENT ELECTROCUTED: FOREGROUND TEST EXECUTION DETECTED\n" +
		"Target Persona: [PRE-SESSION/OS-LEVEL]\n" +
		"User Mandate Violation: All tests MUST be submitted as a scheduler_job (e.g., zqk scheduler scan-tests).\n" +
		"Direct 'go test' execution is strictly forbidden.\n" +
		"Bypass (Make/CI): run under make (MAKEFLAGS), or set " + AllowForegroundGoTest() + "=1.\n" +
		"Do NOT set " + TestRoot() + " for this: it relocates the project root, so every later\n" +
		"zqk command in the same shell resolves against that directory and fails."
}

func isGoTestBinary(arg0 string) bool {
	return strings.HasSuffix(arg0, ".test") ||
		strings.Contains(arg0, "/_test/") ||
		strings.Contains(arg0, "\\_test\\")
}

// foregroundGoTestAllowed reports whether the current go-test process is an
// allowed context (scheduler, isolated TEST_ROOT, Make, CI, or explicit opt-in).
func foregroundGoTestAllowed() bool {
	// Scheduler run_wrapper sets brand JOB_ID. Also accept studio ZQK_* aliases and
	// the historical misnamed ZQK_SCHEDULER_JOB_ID used by run-tests-bg.sh.
	if anyEnvSet(JobID(), "ZQK_JOB_ID", "ZQK_SCHEDULER_JOB_ID") {
		return true
	}
	// Purpose-named opt-in: grants the permission without moving any path.
	if envTruthy(AllowForegroundGoTest()) || envValueTruthy(os.Getenv("ZQK_ALLOW_FOREGROUND_GO_TEST")) {
		return true
	}
	// TestRoot is accepted because scheduler bundlers and local CI already export it, so removing it
	// here would start failing runs that are legitimately isolated. It is not the recommended bypass
	// (see the panic text): it grants this permission only incidentally, as a side effect of naming a
	// location, and that coupling is what makes it unsafe to suggest.
	if anyEnvSet(TestRoot(), "ZQK_TEST_ROOT") {
		return true
	}
	// In-package test harness marker.
	if v := os.Getenv(InTest()); v == "true" || v == "1" {
		return true
	}
	// Help golden updates (make generate-spec-builders).
	if os.Getenv(UpdateHelpGolden()) == "1" || os.Getenv("ZQKCLI_TEST_UPDATE_HELP_GOLDEN") == "1" {
		return true
	}

	// Make recipes always set MAKEFLAGS (and usually MAKELEVEL).
	if os.Getenv("MAKEFLAGS") != "" || os.Getenv("MAKELEVEL") != "" {
		return true
	}
	// Hosted CI.
	if envValueTruthy(os.Getenv("CI")) || os.Getenv("GITHUB_ACTIONS") != "" || os.Getenv("GITLAB_CI") != "" {
		return true
	}
	return false
}

func anyEnvSet(keys ...string) bool {
	for _, k := range keys {
		if k != "" && os.Getenv(k) != "" {
			return true
		}
	}
	return false
}

func envTruthy(key string) bool {
	return envValueTruthy(os.Getenv(key))
}

func envValueTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
