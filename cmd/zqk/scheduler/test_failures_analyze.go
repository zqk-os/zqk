package scheduler

import (
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
)

// TestFailureStats tracks statistics for a specific test failure
type TestFailureStats struct {
	TestName      string
	PackagePath   string
	FailureCount  int
	TotalRuns     int
	FailureRate   float64
	FirstSeen     time.Time
	LastSeen      time.Time
	ErrorMessages []string
}

// analyzeTestFailures orchestrates test failure log analysis and optional backlog item creation.
// It uses the pipeline API (in test_failures_analyze_pipeline.go) for stage observability.
func analyzeTestFailures(cliCtx *cli.Context, cmd *cobra.Command) error {
	return analyzeTestFailuresViaPipeline(cliCtx, cmd)
}
