package scheduler

import (
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/gotestparse"
)

func TestFingerprintBundleCommand_stripsLogPaths(t *testing.T) {
	a := FingerprintBundleCommand("/bin/sh -c go test ./pkg/foo -run X > /Users/x/.zqk/logs/scheduler/cvs/test-bundles/bundle-1.log 2>&1")
	b := FingerprintBundleCommand("/bin/sh -c go test ./pkg/foo -run X > /other/.zqk/logs/scheduler/cvs/test-bundles/bundle-2.log 2>&1")
	if a != b {
		t.Fatalf("expected same fingerprint after normalizing log paths, got %q vs %q", a, b)
	}
}

func TestFormatRunWrapperCommandString(t *testing.T) {
	// Log paths under .zqk/logs/ are normalized by FingerprintBundleCommand (same as run_wrapper cmdStr).
	a := FormatRunWrapperCommandString("/bin/sh", []string{"-c", "go test ./pkg/foo -run X > /x/.zqk/logs/scheduler/cvs/test-bundles/a.log 2>&1"})
	b := FormatRunWrapperCommandString("/bin/sh", []string{"-c", "go test ./pkg/foo -run X > /y/.zqk/logs/scheduler/cvs/test-bundles/b.log 2>&1"})
	if FingerprintBundleCommand(a) != FingerprintBundleCommand(b) {
		t.Fatalf("fingerprints should match when only .zqk/logs path differs")
	}
	if !strings.Contains(a, "/bin/sh") || !strings.Contains(a, "-c") {
		t.Fatalf("unexpected formatted command: %s", a)
	}
}

func TestBuildSuggestedGoTestRerunCommands(t *testing.T) {
	cmdStr := `go test ./cmd/zqk/system -run '^(TestA|TestB)$' -v -count=1 -timeout 120s -p 10`
	got := BuildSuggestedGoTestRerunCommands(cmdStr, "go", []string{
		"test", "./cmd/zqk/system", "-run", "^(TestA|TestB)$", "-v", "-count=1", "-timeout", "120s", "-p", "10",
	}, []string{
		"github.com/zqk-os/zqk/cmd/zqk/system.TestA",
		"cmd/zqk/system.TestB",
	}, 600)
	if len(got) != 1 {
		t.Fatalf("want 1 command, got %d: %v", len(got), got)
	}
	if !strings.Contains(got[0], "go test ./cmd/zqk/system") {
		t.Errorf("missing package: %s", got[0])
	}
	if !strings.Contains(got[0], "-run") {
		t.Errorf("missing -run: %s", got[0])
	}
	if !strings.Contains(got[0], "TestA") || !strings.Contains(got[0], "TestB") {
		t.Errorf("missing test names: %s", got[0])
	}
}

func TestSplitFailedTestName(t *testing.T) {
	pkg, fn := splitFailedTestName("github.com/zqk-os/zqk/pkg/storage.TestCAS", "pkg/storage")
	if pkg != "pkg/storage" || fn != "TestCAS" {
		t.Fatalf("got %q %q", pkg, fn)
	}
	pkg2, fn2 := splitFailedTestName("TestOnly", "cmd/zqk")
	if pkg2 != "cmd/zqk" || fn2 != "TestOnly" {
		t.Fatalf("got %q %q", pkg2, fn2)
	}
}

func TestTrimToLastSchedulerTestRunForParsing_ParseGoTestOutput(t *testing.T) {
	// Same shape as appended SCH-run-* stdout: multiple "--- run RFC3339 ---" blocks; parser must not count FAIL from an earlier block.
	const multi = `
--- run 2026-03-22T17:33:52Z ---
--- FAIL: TestAllKindsCRUD (0.48s)
FAIL
FAIL	github.com/zqk-os/zqk/pkg/storage	86.399s
FAIL

--- run 2026-03-22T18:07:20Z ---
--- PASS: github.com/zqk-os/zqk/pkg/storage.TestOk (0.01s)
PASS
ok  	github.com/zqk-os/zqk/pkg/storage	1.725s
`
	full := "\n" + strings.TrimSpace(multi)
	sumFull, err := gotestparse.ParseGoTestOutput(full)
	if err != nil {
		t.Fatalf("parse full: %v", err)
	}
	if sumFull.FailedCount == 0 {
		t.Fatal("full output should include a failed test from the first run")
	}
	lastOnly := trimToLastSchedulerTestRunForParsing(full)
	sumLast, err := gotestparse.ParseGoTestOutput(lastOnly)
	if err != nil {
		t.Fatalf("parse last run: %v", err)
	}
	if sumLast.FailedCount != 0 {
		t.Fatalf("last run only: want FailedCount 0, got %d (failed: %v)", sumLast.FailedCount, sumLast.FailedTestList)
	}
}

func TestTrimToLastSchedulerTestRunForParsing_skipsEmptyTrailingRunMarkers(t *testing.T) {
	// Real bundle logs can append duplicate bare "--- run RFC3339 ---" lines after the real run; LastIndex
	// on those would leave nothing to parse and health.jsonl would incorrectly show tests_failed=0.
	const multi = `
--- run 2026-03-30T01:46:12Z ---
=== RUN   TestHashMismatchFixEventViaCoordinator
--- FAIL: TestHashMismatchFixEventViaCoordinator (1.00s)
FAIL
FAIL	github.com/zqk-os/zqk/cmd/zqk/system	27.572s
FAIL

--- run 2026-03-30T00:46:10Z ---

--- run 2026-03-30T01:46:12Z ---
`
	full := "\n" + strings.TrimSpace(multi)
	lastOnly := trimToLastSchedulerTestRunForParsing(full)
	sumLast, err := gotestparse.ParseGoTestOutput(lastOnly)
	if err != nil {
		t.Fatalf("parse last substantive run: %v", err)
	}
	if sumLast.FailedCount != 1 {
		t.Fatalf("want FailedCount 1, got %d (last block: %q)", sumLast.FailedCount, lastOnly)
	}
}
