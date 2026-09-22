package gotestparse

import (
	"reflect"
	"testing"
	"time"
)

func TestParseGoTestOutput_ExtraEdgeCases(t *testing.T) {
	t.Parallel()

	// 1. Test GetFailedTestsByPackage and GetFailedTestNames with package prefixes
	summary := &TestRunSummary{
		FailedTestList: []TestResult{
			{
				PackagePath: "pkg/foo",
				TestName:    "TestA",
				Status:      "FAIL",
			},
			{
				PackagePath: "pkg/bar",
				TestName:    "TestB",
				Status:      "FAIL",
			},
			{
				PackagePath: "",
				TestName:    "TestBare",
				Status:      "FAIL",
			},
		},
	}

	names := summary.GetFailedTestNames()
	expectedNames := []string{"pkg/foo.TestA", "pkg/bar.TestB", "TestBare"}
	if !reflect.DeepEqual(names, expectedNames) {
		t.Fatalf("unexpected failed test names: got %v, want %v", names, expectedNames)
	}

	byPkg := summary.GetFailedTestsByPackage()
	if len(byPkg["pkg/foo"]) != 1 || byPkg["pkg/foo"][0].TestName != "TestA" {
		t.Fatalf("unexpected foo package failures: %v", byPkg["pkg/foo"])
	}
	if len(byPkg["pkg/bar"]) != 1 || byPkg["pkg/bar"][0].TestName != "TestB" {
		t.Fatalf("unexpected bar package failures: %v", byPkg["pkg/bar"])
	}
	if len(byPkg[""]) != 1 || byPkg[""][0].TestName != "TestBare" {
		t.Fatalf("unexpected bare package failures: %v", byPkg[""])
	}

	// 2. Data race with existing failed test match
	raceOutput := `=== RUN   TestRace
WARNING: DATA RACE
Write at 0x0001 by goroutine 7:
  foo.go:10
Previous read at 0x0001 by goroutine 8:
  foo.go:15
==================
--- FAIL: pkg/mod.TestRace (0.05s)
    race_test.go:20: failed
FAIL
Found 1 data race(s)
`
	res, err := ParseGoTestOutput(raceOutput)
	if err != nil {
		t.Fatalf("ParseGoTestOutput failed: %v", err)
	}
	if !res.HasDataRace {
		t.Fatalf("expected HasDataRace=true")
	}
	if res.DataRaceCount != 1 {
		t.Fatalf("expected DataRaceCount=1, got %d", res.DataRaceCount)
	}
	if len(res.FailedTestList) != 1 || !res.FailedTestList[0].HasDataRace {
		t.Fatalf("expected test result HasDataRace=true, got %+v", res.FailedTestList)
	}

	// 3. Data race when inRaceBlock extends to end of file without delimiter
	unclosedRaceOutput := `=== RUN   TestUnclosedRace
--- FAIL: TestUnclosedRace (0.01s)
WARNING: DATA RACE
Write at 0x0001 by goroutine 1:
  bar.go:10
`
	resUnclosed, err := ParseGoTestOutput(unclosedRaceOutput)
	if err != nil {
		t.Fatalf("ParseGoTestOutput failed: %v", err)
	}
	if !resUnclosed.HasDataRace {
		t.Fatalf("expected HasDataRace=true for unclosed race")
	}


	// 5. In-failure block ending via goBoundaryPackageOK or statusFail
	failBoundaryOutput := `=== RUN   TestFailBoundary
--- FAIL: TestFailBoundary (0.02s)
    some log line
FAIL
ok  pkg/other 0.10s
`
	resBoundary, err := ParseGoTestOutput(failBoundaryOutput)
	if err != nil {
		t.Fatalf("ParseGoTestOutput failed: %v", err)
	}
	if len(resBoundary.FailedTestList) != 1 {
		t.Fatalf("expected 1 failed test, got %d", len(resBoundary.FailedTestList))
	}

	// 6. applyPackageResultLineFallback with nil summary
	applyPackageResultLineFallback(nil, "ok pkg 0.1s")

	// 7. applyPackageResultLineFallback with non-matching lines and empty lines
	fallbackSummary := &TestRunSummary{}
	applyPackageResultLineFallback(fallbackSummary, "\n   \n--- not a pkg line\nsome random text\n")
	if fallbackSummary.TotalTests != 0 {
		t.Fatalf("expected 0 total tests from non-matching fallback, got %d", fallbackSummary.TotalTests)
	}

	// 8. Duration accumulation from package pass line
	pkgPassOutput := `=== RUN   TestOne
--- PASS: TestOne (0.01s)
PASS
ok  example.com/mod 1.500s
`
	resPkgPass, err := ParseGoTestOutput(pkgPassOutput)
	if err != nil {
		t.Fatalf("ParseGoTestOutput failed: %v", err)
	}
	if resPkgPass.Duration < time.Second {
		t.Fatalf("expected duration >= 1s, got %v", resPkgPass.Duration)
	}
}
