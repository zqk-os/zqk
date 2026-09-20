package testing

import "github.com/zqk-os/zqk/pkg/gotestparse"

// TestResult is re-exported from [github.com/zqk-os/zqk/pkg/gotestparse].
type TestResult = gotestparse.TestResult

// TestRunSummary is re-exported from [github.com/zqk-os/zqk/pkg/gotestparse].
type TestRunSummary = gotestparse.TestRunSummary

// ParseGoTestOutput parses go test output and extracts test results.
func ParseGoTestOutput(output string) (*TestRunSummary, error) {
	return gotestparse.ParseGoTestOutput(output)
}
