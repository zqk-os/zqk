package gotestparse

import (
	"bufio"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

const (
	statusPass             = "PASS"
	statusFail             = "FAIL"
	statusSkip             = "SKIP"
	goDurationSuffix       = "s"
	goBoundaryTestPrefix   = "---"
	goBoundaryPackageOK    = "ok"
	goFailureJoinSeparator = "\n"
	errParseOutputFmt      = "failed to parse test output: %w"
)

// DataRaceIncident captures a data race event reported by the Go race detector.
type DataRaceIncident struct {
	TestName    string `json:"test_name,omitempty"`
	PackagePath string `json:"package_path,omitempty"`
	Description string `json:"description"`
	Details     string `json:"details"`
}

// TestResult represents the result of a test run.
type TestResult struct {
	PackagePath string
	TestName    string
	Status      string // "PASS", "FAIL", "SKIP"
	Duration    time.Duration
	Error       string
	HasDataRace bool
}

// TestRunSummary represents a summary of a test run.
type TestRunSummary struct {
	TotalTests        int
	PassedCount       int
	FailedCount       int
	SkippedCount      int
	Duration          time.Duration
	FailedTestList    []TestResult
	PassedTestList    []TestResult
	SkippedTestList   []TestResult
	HasDataRace       bool
	DataRaceCount     int
	DataRaceIncidents []DataRaceIncident
}

// ParseGoTestOutput parses go test output and extracts test results.
//
//nolint:gocyclo // Test output parsing requires multiple pattern matching branches
func ParseGoTestOutput(output string) (*TestRunSummary, error) {
	summary := &TestRunSummary{
		FailedTestList:  []TestResult{},
		PassedTestList:  []TestResult{},
		SkippedTestList: []TestResult{},
	}

	scanner := bufio.NewScanner(strings.NewReader(output))

	passPattern := regexp.MustCompile(`^--- PASS:\s+(\S+)\s+\(([0-9.]+)s\)`)
	failPattern := regexp.MustCompile(`^--- FAIL:\s+(\S+)`)
	skipPattern := regexp.MustCompile(`^--- SKIP:\s+(\S+)`)
	runPattern := regexp.MustCompile(`^=== RUN\s+(\S+)`)
	packagePassPattern := regexp.MustCompile(`^ok\s+(\S+)\s+([0-9.]+)s`)
	foundRacePattern := regexp.MustCompile(`Found (\d+) data race\(s\)`)

	var currentTest *TestResult
	var inFailureBlock bool
	var failureLines []string
	var activeTestName string
	var inRaceBlock bool
	var raceLines []string

	for scanner.Scan() {
		line := scanner.Text()

		if match := packagePassPattern.FindStringSubmatch(line); match != nil {
			duration, err := time.ParseDuration(match[2] + goDurationSuffix)
			if err == nil {
				summary.Duration += duration
			}
		}

		if match := runPattern.FindStringSubmatch(line); match != nil {
			activeTestName = match[1]
		}

		if match := foundRacePattern.FindStringSubmatch(line); match != nil {
			summary.HasDataRace = true
		}

		if strings.Contains(line, "WARNING: DATA RACE") {
			inRaceBlock = true
			summary.HasDataRace = true
			raceLines = []string{line}
		} else if inRaceBlock {
			raceLines = append(raceLines, line)
			if strings.TrimSpace(line) == "==================" && len(raceLines) > 2 {
				incident := DataRaceIncident{
					TestName:    activeTestName,
					Description: "WARNING: DATA RACE",
					Details:     strings.Join(raceLines, "\n"),
				}
				summary.DataRaceIncidents = append(summary.DataRaceIncidents, incident)
				summary.DataRaceCount++
				if currentTest != nil {
					currentTest.HasDataRace = true
				} else {
					for i := range summary.FailedTestList {
						if summary.FailedTestList[i].TestName == activeTestName {
							summary.FailedTestList[i].HasDataRace = true
						}
					}
				}
				inRaceBlock = false
				raceLines = nil
			}
		}

		if match := failPattern.FindStringSubmatch(line); match != nil {
			testName := match[1]
			pkg := ""
			test := testName
			if parts := strings.Split(testName, "."); len(parts) >= 2 {
				pkg = strings.Join(parts[:len(parts)-1], ".")
				test = parts[len(parts)-1]
			}
			hasRace := false
			for _, inc := range summary.DataRaceIncidents {
				if inc.TestName == test || inc.TestName == testName {
					hasRace = true
					break
				}
			}
			currentTest = &TestResult{
				PackagePath: pkg,
				TestName:    test,
				Status:      statusFail,
				HasDataRace: hasRace,
			}
			inFailureBlock = true
			failureLines = []string{}

			summary.FailedTestList = append(summary.FailedTestList, *currentTest)
			summary.TotalTests++
			summary.FailedCount++
		} else if match := passPattern.FindStringSubmatch(line); match != nil {
			testName := match[1]
			duration, err := time.ParseDuration(match[2] + goDurationSuffix)
			if err != nil {
				duration = 0
			}
			pkg := ""
			test := testName
			if parts := strings.Split(testName, "."); len(parts) >= 2 {
				pkg = strings.Join(parts[:len(parts)-1], ".")
				test = parts[len(parts)-1]
			}
			summary.PassedTestList = append(summary.PassedTestList, TestResult{
				PackagePath: pkg,
				TestName:    test,
				Status:      statusPass,
				Duration:    duration,
			})
			summary.TotalTests++
			summary.PassedCount++
			inFailureBlock = false
		} else if match := skipPattern.FindStringSubmatch(line); match != nil {
			testName := match[1]
			pkg := ""
			test := testName
			if parts := strings.Split(testName, "."); len(parts) >= 2 {
				pkg = strings.Join(parts[:len(parts)-1], ".")
				test = parts[len(parts)-1]
			}
			summary.SkippedTestList = append(summary.SkippedTestList, TestResult{
				PackagePath: pkg,
				TestName:    test,
				Status:      statusSkip,
			})
			summary.TotalTests++
			summary.SkippedCount++
			inFailureBlock = false
		} else if inFailureBlock && currentTest != nil {
			failureLines = append(failureLines, line)
			if strings.HasPrefix(strings.TrimSpace(line), goBoundaryTestPrefix) ||
				strings.HasPrefix(strings.TrimSpace(line), goBoundaryPackageOK) ||
				strings.HasPrefix(strings.TrimSpace(line), statusFail) {
				if len(summary.FailedTestList) > 0 {
					lastIdx := len(summary.FailedTestList) - 1
					summary.FailedTestList[lastIdx].Error = strings.Join(failureLines, goFailureJoinSeparator)
				}
				inFailureBlock = false
			}
		}
	}

	if inRaceBlock && len(raceLines) > 0 {
		incident := DataRaceIncident{
			TestName:    activeTestName,
			Description: "WARNING: DATA RACE",
			Details:     strings.Join(raceLines, "\n"),
		}
		summary.DataRaceIncidents = append(summary.DataRaceIncidents, incident)
		summary.DataRaceCount++
		if currentTest != nil {
			currentTest.HasDataRace = true
		}
	}

	if inFailureBlock && len(failureLines) > 0 && len(summary.FailedTestList) > 0 {
		lastIdx := len(summary.FailedTestList) - 1
		summary.FailedTestList[lastIdx].Error = strings.Join(failureLines, goFailureJoinSeparator)
	}

	if err := scanner.Err(); err != nil {
		return nil, errfmt.Errorf(errParseOutputFmt, err)
	}

	// Non-verbose `go test` often omits --- PASS lines and only prints `ok  <import>  0.05s` or `(cached)`.
	// Those lines were used for duration above but did not increment per-test counts — synthesize package-level
	// counts so scheduler test_summary / chat snippets are not all zeros on green runs.
	if summary.TotalTests == 0 {
		applyPackageResultLineFallback(summary, output)
	}

	return summary, nil
}

// rePackageResultOK matches `ok  	import/path	0.05s` or `...	(cached)` (non-verbose package pass).
// rePackageResultFAIL matches final `FAIL	import/path	0.05s` when there was no --- FAIL line.
var (
	rePackageResultOK   = regexp.MustCompile(`^\s*ok\s+(\S+)\s+`)
	rePackageResultFAIL = regexp.MustCompile(`^\s*FAIL\s+(\S+)\s+`)
)

func applyPackageResultLineFallback(summary *TestRunSummary, output string) {
	if summary == nil {
		return
	}
	for _, line := range strings.Split(output, "\n") {
		s := strings.TrimSpace(line)
		if s == "" {
			continue
		}
		if strings.HasPrefix(s, "--- ") {
			continue
		}
		if rePackageResultOK.MatchString(s) {
			summary.TotalTests++
			summary.PassedCount++
			continue
		}
		if rePackageResultFAIL.MatchString(s) {
			summary.TotalTests++
			summary.FailedCount++
		}
	}
}

// GetFailedTestNames returns a list of failed test names in format "package.TestName"
func (s *TestRunSummary) GetFailedTestNames() []string {
	names := make([]string, 0, len(s.FailedTestList))
	for _, test := range s.FailedTestList {
		fullName := test.TestName
		if test.PackagePath != "" {
			fullName = fmt.Sprintf("%s.%s", test.PackagePath, test.TestName)
		}
		names = append(names, fullName)
	}
	return names
}

// GetFailedTestsByPackage groups failed tests by package.
func (s *TestRunSummary) GetFailedTestsByPackage() map[string][]TestResult {
	byPackage := make(map[string][]TestResult)
	for _, test := range s.FailedTestList {
		byPackage[test.PackagePath] = append(byPackage[test.PackagePath], test)
	}
	return byPackage
}
