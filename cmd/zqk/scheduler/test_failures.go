package scheduler

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	schedpkg "github.com/zqk-os/zqk/pkg/scheduler"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// ============================================================================
// Extracted Modules
// ============================================================================
//
// This file has been split into focused modules for better maintainability:
//
// - Command creation -> test_failures_commands.go
//   - NewTestFailuresCmd, NewTestFailuresListCmd, NewTestFailuresRerunCmd, NewTestFailuresAnalyzeCmd, NewTestFailuresHealthCmd
//
// - List operations -> test_failures_list.go
//   - listTestFailures (processes callback logs and scheduler logs to find failures)
//
// - Rerun operations -> test_failures_rerun.go
//   - rerunTestFailures (collects failures and schedules new test jobs)
//
// - Analyze operations -> test_failures_analyze.go
//   - TestFailureStats type, analyzeTestFailures (analyzes failure patterns and creates backlog items)
//
// - Health timeline -> test_failures_health.go
//   - runTestFailuresHealth (uses readTestBundleHealthTailForCLI)
//
// - Convergence measure -> test_failures_convergence.go, scheduler_convergence_from_spec.go (YAML + CommandSpecBuilder)
//   - runTestFailuresConvergence (delta_assessment + heartbeat from health.jsonl)
//
// Command spec: .zqk/cli/specs/scheduler/test_failures_command.yaml (generate-command-builders)
//
// Original file: 1,044 lines
// After split: This file now serves as documentation and module index

// User-facing message when test-bundles/health.jsonl is absent (health + convergence subcommands).
const msgTestBundleHealthFileMissing = "No test health file yet. Run zqk test run (kernel test_case objects).\n"

// errTestBundleHealthFileHandled is returned when the missing-file message was already written to cmd.
var errTestBundleHealthFileHandled = errfmt.Errorf("test-bundle health.jsonl missing (user message already written)")

// readTestBundleHealthTailForCLI loads the tail of health.jsonl. If the file is missing, writes
// msgTestBundleHealthFileMissing to cmd and returns (nil, errTestBundleHealthFileHandled).
func readTestBundleHealthTailForCLI(cmd *cobra.Command, projectRoot string, limit int) ([]map[string]any, error) {
	rctx := context.Background() // Background: request-or-shutdown derived
	if c := cmd.Context(); c != nil {
		rctx = c
	}
	lines, err := schedpkg.ReadTestBundleHealthTailLines(rctx, projectRoot, limit)
	if err != nil {
		if fileutil.IsNotExist(err) {
			format := string(cli.GetFormat(cmd))
			if format == "json" || format == "yaml" {
				return []map[string]any{}, nil
			}
			if werr := cli.WriteOutput(cmd, []byte(msgTestBundleHealthFileMissing)); werr != nil {
				return nil, werr
			}
			return nil, errTestBundleHealthFileHandled
		}
		return nil, errfmt.Newf("read health log").Wrap(err)
	}
	return lines, nil
}

// readTestBundleHealthTailForAgentPrompt loads health.jsonl like [readTestBundleHealthTailForCLI] but
// returns an empty window when the file is missing so --format agent-prompt can still emit CVS-backed
// markdown (measurement section reflects unknown / no lines). Other formats keep the
// user-facing “run test bundles first” message via [readTestBundleHealthTailForCLI].
func readTestBundleHealthTailForAgentPrompt(projectRoot string, limit int) ([]map[string]any, error) {
	lines, err := schedpkg.ReadTestBundleHealthTailLines(context.Background(), projectRoot, limit) // Background: request-or-shutdown derived
	if err != nil && fileutil.IsNotExist(err) {
		return []map[string]any{}, nil
	}
	return lines, err
}

type testFailuresFilterConfig struct {
	ProjectRoot   string
	PackageFilter string
	Since         time.Duration
	CutoffTime    time.Time
}

func parseTestFailuresFilterConfig(cliCtx *cli.Context, cmd *cobra.Command) (*testFailuresFilterConfig, error) {
	_ = cliCtx
	projectRoot, err := resolveSchedulerProjectRoot(cmd)
	if err != nil {
		return nil, err
	}

	packageFilter, _ := cmd.Flags().GetString("package")
	sinceStr, _ := cmd.Flags().GetString("since")

	since, err := time.ParseDuration(sinceStr)
	if err != nil {
		return nil, errfmt.Errorf("invalid duration %q: %w", sinceStr, err)
	}

	return &testFailuresFilterConfig{
		ProjectRoot:   projectRoot,
		PackageFilter: packageFilter,
		Since:         since,
		CutoffTime:    time.Now().Add(-since),
	}, nil
}

func parseCallbackTestFailures(entry map[string]any, packageFilter string, fn func(pkg, testName string)) {
	eventType, _ := entry[objects.FieldKeyEventType].(string)
	if eventType != schedulerStateFailed && eventType != schedulerStateError {
		return
	}

	testFailures, ok := entry["test_failures"].([]any)
	if !ok {
		return
	}

	for _, failure := range testFailures {
		failureStr, ok := failure.(string)
		if !ok {
			continue
		}

		parts := strings.Split(failureStr, ".")
		if len(parts) < 2 {
			continue
		}

		pkg := strings.Join(parts[:len(parts)-1], ".")
		testName := parts[len(parts)-1]

		if packageFilter != emptyValue && !strings.Contains(pkg, packageFilter) {
			continue
		}

		pkg = strings.TrimPrefix(pkg, "github.com/zqk-os/zqk/")
		if !strings.HasPrefix(pkg, "./") {
			pkg = "./" + pkg
		}

		fn(pkg, testName)
	}
}

func walkLogFileLines(dir string, cutoff time.Time, allowedExts []string, fn func(line string)) error {
	if _, err := fileutil.Stat(dir); err != nil {
		return nil
	}
	return filepath.Walk(dir, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil || info.ModTime().Before(cutoff) {
			return nil
		}
		hasExt := false
		for _, ext := range allowedExts {
			if strings.HasSuffix(path, ext) {
				hasExt = true
				break
			}
		}
		if !hasExt {
			return nil
		}
		data, err := fileutil.ReadFile(path)
		if err != nil {
			return nil
		}
		for line := range strings.SplitSeq(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line != emptyValue {
				fn(line)
			}
		}
		return nil
	})
}
