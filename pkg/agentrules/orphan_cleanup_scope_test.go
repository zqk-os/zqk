package agentrules

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const orphanCleanupScript = "scripts/cleanup_orphan_test_processes.sh"

// daemonPgrepPattern pulls the pattern the script hands to pgrep for "abandoned daemon" candidates,
// so this test exercises the shipped line rather than a copy that can drift away from it.
func daemonPgrepPattern(t *testing.T) *regexp.Regexp {
	t.Helper()
	data, err := fileutil.ReadFile(filepath.Join(repoRoot(t), orphanCleanupScript))
	if err != nil {
		t.Fatalf("read %s: %v", orphanCleanupScript, err)
	}
	// ZQK_DAEMON_PIDS=$(pgrep -f "<pattern>" ...)
	assign := regexp.MustCompile(`ZQK_DAEMON_PIDS=\$\(pgrep -f "([^"]+)"`)
	m := assign.FindSubmatch(data)
	if m == nil {
		t.Fatalf("could not find the ZQK_DAEMON_PIDS pgrep pattern in %s; if the variable was "+
			"renamed, update this test rather than deleting it", orphanCleanupScript)
	}
	re, err := regexp.Compile(string(m[1]))
	if err != nil {
		t.Fatalf("pattern %q does not compile: %v", m[1], err)
	}
	return re
}

// TestOrphanCleanupDoesNotClassifySchedulerSubcommandsAsDaemons pins the blast radius of the --kill
// pass that scripts/agent-validate-changes.sh makes from the pre-commit gates.
//
// The pattern used to be ".*scheduler", which matches every one-off `zqk scheduler …` invocation.
// A `scheduler scan-tests` run dispatched as a scheduler job was therefore listed as an abandoned
// daemon and SIGKILLed 438s into an 1800s budget, immediately after planning 143 bundles — no
// timeout, no memory pressure (95% free, swap unused), just a pattern that was too wide. The
// failure mode is quiet: the job dies with signal: killed and the packages it was scanning never
// report an outcome at all.
func TestOrphanCleanupDoesNotClassifySchedulerSubcommandsAsDaemons(t *testing.T) {
	re := daemonPgrepPattern(t)

	// Long-lived processes: killing these when truly abandoned is the point of the script.
	daemonArgv := []string{
		"/Users/x/zqk/.zqk/bin/zqk-stable scheduler start --foreground",
		"./bin/zqk-stable scheduler daemon",
		"bin/zqk scheduler start",
	}
	// One-off CLI work: bounded, usually supervised, and never a leaked daemon.
	transientArgv := []string{
		"./bin/zqk-stable scheduler scan-tests --package ./pkg/objects",
		"./bin/zqk-stable scheduler submit ./script.sh --title t",
		"./bin/zqk-stable scheduler history --job-id SCH-1787601142198539000",
		"./bin/zqk-stable scheduler activity",
		"./bin/zqk-stable scheduler test-failures",
		"./bin/zqk-stable scheduler convergence measure --session-id CVS-1",
	}

	for _, argv := range daemonArgv {
		if !re.MatchString(argv) {
			t.Errorf("daemon no longer recognized, so a genuinely leaked daemon will survive "+
				"cleanup: %q", argv)
		}
	}
	for _, argv := range transientArgv {
		if re.MatchString(argv) {
			t.Errorf("one-off scheduler subcommand is classified as an abandoned daemon and will be "+
				"SIGKILLed mid-run by the pre-commit --kill pass: %q", argv)
		}
	}

	// The pattern must also stay anchored at argv[0]. Unanchoring it matches command lines that
	// merely name the package, and `go build ./cmd/zqk/agent ./pkg/scheduler` is the pre-commit's
	// own compile step.
	if re.MatchString("go build ./cmd/zqk/agent ./pkg/scheduler start") {
		t.Error("pattern matches a command line that only names the scheduler package; it must stay " +
			"anchored at argv[0] or cleanup will kill the pre-commit's own compile step")
	}
}

// TestOrphanCleanupExemptsDaemonSupervisedWork guards the second half of the same defect: candidates
// are also gathered with pgrep -f "go test", which matches the bundle tests the live daemon is
// running. Only the daemon's own pid was exempt, so running the gates during a scan killed the
// bundles it had in flight, and a killed bundle writes no health.jsonl outcome — the package simply
// never reports, which reads as "still running" rather than as a failure.
func TestOrphanCleanupExemptsDaemonSupervisedWork(t *testing.T) {
	data, err := fileutil.ReadFile(filepath.Join(repoRoot(t), orphanCleanupScript))
	if err != nil {
		t.Fatalf("read %s: %v", orphanCleanupScript, err)
	}
	body := string(data)

	if !strings.Contains(body, "pid_descends_from()") {
		t.Fatal("pid_descends_from helper is gone; without it the only scheduler exemption is the " +
			"daemon's own pid, and every test the daemon is running is a kill candidate")
	}
	if !strings.Contains(body, `pid_descends_from "$pid" "$MAIN_SCHEDULER_PID"`) {
		t.Error("the candidate loop no longer exempts descendants of the running scheduler daemon; " +
			"supervised work has a live parent and is not orphaned")
	}
	// The walk must terminate: a ps hiccup returning the candidate's own pid, or a cycle, must not
	// spin inside a pre-commit gate.
	helper := body[strings.Index(body, "pid_descends_from()"):]
	if end := strings.Index(helper, "\n}\n"); end > 0 {
		helper = helper[:end]
	}
	if !strings.Contains(helper, "_pdf_depth") {
		t.Error("pid_descends_from has no depth bound; the parent walk must terminate even if ps " +
			"returns an unexpected chain")
	}
}
