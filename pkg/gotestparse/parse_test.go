package gotestparse

import "testing"

func TestParseGoTestOutput_FailWithoutPackagePrefix(t *testing.T) {
	t.Parallel()

	out := `=== RUN   TestSomething
--- FAIL: TestSomething (0.01s)
    foo_test.go:12: boom
FAIL
exit status 1
FAIL	example.com/mod/pkg	0.123s
`

	summary, err := ParseGoTestOutput(out)
	if err != nil {
		t.Fatalf("ParseGoTestOutput returned error: %v", err)
	}
	if summary == nil {
		t.Fatalf("expected non-nil summary")
	}
	if summary.FailedCount != 1 {
		t.Fatalf("expected FailedCount=1, got %d", summary.FailedCount)
	}
	failed := summary.GetFailedTestNames()
	if len(failed) != 1 || failed[0] != "TestSomething" {
		t.Fatalf("expected failed tests [TestSomething], got %#v", failed)
	}
}

func TestParseGoTestOutput_NonVerbosePackageOkLine(t *testing.T) {
	t.Parallel()
	out := `PASS
ok  	example.com/mod/pkg	0.050s
`
	summary, err := ParseGoTestOutput(out)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if summary.TotalTests != 1 || summary.PassedCount != 1 || summary.FailedCount != 0 {
		t.Fatalf("want 1 pass at package level, got total=%d passed=%d failed=%d",
			summary.TotalTests, summary.PassedCount, summary.FailedCount)
	}
}

func TestParseGoTestOutput_NonVerbosePackageCached(t *testing.T) {
	t.Parallel()
	out := `ok  	example.com/mod/pkg	(cached)
`
	summary, err := ParseGoTestOutput(out)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if summary.TotalTests != 1 || summary.PassedCount != 1 {
		t.Fatalf("want 1 package pass for (cached), got total=%d passed=%d", summary.TotalTests, summary.PassedCount)
	}
}

func TestParseGoTestOutput_NonVerbosePackageFailLine(t *testing.T) {
	t.Parallel()
	out := `FAIL	example.com/mod/pkg	0.030s
`
	summary, err := ParseGoTestOutput(out)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if summary.TotalTests != 1 || summary.FailedCount != 1 || summary.PassedCount != 0 {
		t.Fatalf("want 1 package fail, got total=%d passed=%d failed=%d",
			summary.TotalTests, summary.PassedCount, summary.FailedCount)
	}
}

func TestParseGoTestOutput_VerbosePassBareFunctionName(t *testing.T) {
	t.Parallel()
	out := `=== RUN   TestContext_PathResolver
--- PASS: TestContext_PathResolver (0.01s)
=== RUN   TestContext_WithPathResolver
--- PASS: TestContext_WithPathResolver (0.02s)
PASS
ok  	example.com/mod/internal/cli/context	1.143s
`
	summary, err := ParseGoTestOutput(out)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if summary.TotalTests != 2 || summary.PassedCount != 2 || summary.FailedCount != 0 {
		t.Fatalf("want 2 passes (go test -v uses bare func names), got total=%d passed=%d failed=%d",
			summary.TotalTests, summary.PassedCount, summary.FailedCount)
	}
	if len(summary.PassedTestList) != 2 || summary.PassedTestList[0].TestName != "TestContext_PathResolver" {
		t.Fatalf("unexpected passed list: %#v", summary.PassedTestList)
	}
}

func TestParseGoTestOutput_DataRaceDetection(t *testing.T) {
	t.Parallel()
	out := `=== RUN   TestConcurrentRaceCondition
==================
WARNING: DATA RACE
Write at 0x00c00012e060 by goroutine 7:
  github.com/zqk-os/zqk/pkg/foo.BadFunc()
      /path/to/foo.go:42 +0x34

Previous read at 0x00c00012e060 by goroutine 6:
  github.com/zqk-os/zqk/pkg/foo.ReadFunc()
      /path/to/foo.go:30 +0x28
==================
--- FAIL: TestConcurrentRaceCondition (0.05s)
    testing.go:1312: race detected during execution of test
FAIL
Found 1 data race(s)
FAIL	example.com/mod/pkg	0.150s
`
	summary, err := ParseGoTestOutput(out)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !summary.HasDataRace {
		t.Fatalf("expected HasDataRace=true")
	}
	if summary.DataRaceCount != 1 {
		t.Fatalf("expected DataRaceCount=1, got %d", summary.DataRaceCount)
	}
	if len(summary.DataRaceIncidents) != 1 {
		t.Fatalf("expected 1 incident, got %d", len(summary.DataRaceIncidents))
	}
	inc := summary.DataRaceIncidents[0]
	if inc.TestName != "TestConcurrentRaceCondition" {
		t.Errorf("expected incident test name TestConcurrentRaceCondition, got %s", inc.TestName)
	}
	if summary.FailedCount != 1 {
		t.Errorf("expected FailedCount=1, got %d", summary.FailedCount)
	}
	if len(summary.FailedTestList) > 0 && !summary.FailedTestList[0].HasDataRace {
		t.Errorf("expected FailedTestList[0].HasDataRace=true")
	}
}
