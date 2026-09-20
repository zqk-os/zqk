package testrunner

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
)

// StreamOptions controls execution parameters for test progress streaming.
type StreamOptions struct {
	ProjectRoot string
	Pkg         string
	Parallel    int
	Timeout     time.Duration
}

// StreamSummary captures aggregated test execution outcomes.
type StreamSummary struct {
	Passed   int           `json:"passed" yaml:"passed"`
	Failed   int           `json:"failed" yaml:"failed"`
	Skipped  int           `json:"skipped" yaml:"skipped"`
	Duration time.Duration `json:"duration" yaml:"duration"`
	ExitCode int           `json:"exit_code" yaml:"exit_code"`
}

var (
	reSkip = regexp.MustCompile(`^\?\s+github\.com/zqk-os/zqk/(.+)$`)
	rePass = regexp.MustCompile(`^ok\s+github\.com/zqk-os/zqk/(\S+)\s+([0-9.]+)s`)
	reFail = regexp.MustCompile(`^FAIL\s+github\.com/zqk-os/zqk/(\S+)`)
)

// StreamTests runs tests with real-time progress streaming and output formatting.
func StreamTests(ctx context.Context, opts StreamOptions, out io.Writer) (StreamSummary, error) {
	if opts.Parallel <= 0 {
		opts.Parallel = runtime.NumCPU() * 2
	}
	if opts.Timeout <= 0 {
		opts.Timeout = time.Hour
	}
	if opts.Pkg == "" {
		opts.Pkg = "./..."
	}

	startTime := time.Now()
	timestamp := startTime.Format("20060102_150405")
	logDir := filepath.Join(opts.ProjectRoot, paths.ProjectDataDir, paths.LogsDir)
	_ = os.MkdirAll(logDir, paths.DirPerm755)
	logFile := filepath.Join(logDir, fmt.Sprintf("test-output-%s.txt", timestamp))

	logWriter, err := os.Create(logFile)
	if err != nil {
		return StreamSummary{}, errfmt.Newf("create test log file %q", logFile).Wrap(err)
	}
	defer logWriter.Close()

	fmt.Fprintf(out, "=========================================\n")
	fmt.Fprintf(out, "ZQK Test Runner (Progress Streaming)\n")
	fmt.Fprintf(out, "=========================================\n")
	fmt.Fprintf(out, "Package: %s\n", opts.Pkg)
	fmt.Fprintf(out, "Parallelism: %d\n", opts.Parallel)
	fmt.Fprintf(out, "Log file: %s\n", logFile)
	fmt.Fprintf(out, "=========================================\n\n")

	args := []string{
		"test",
		opts.Pkg,
		fmt.Sprintf("-timeout=%s", opts.Timeout),
		fmt.Sprintf("-p=%d", opts.Parallel),
		"-v",
		"-count=1",
	}

	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = opts.ProjectRoot
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		"DEVELOPER_DIR=/Library/Developer/CommandLineTools",
	)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return StreamSummary{}, errfmt.Newf("open test stdout pipe").Wrap(err)
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return StreamSummary{}, errfmt.Newf("start go test process").Wrap(err)
	}

	var summary StreamSummary
	scanner := bufio.NewScanner(io.TeeReader(stdoutPipe, logWriter))

	for scanner.Scan() {
		line := scanner.Text()
		elapsedSec := int(time.Since(startTime).Seconds())

		if m := reSkip.FindStringSubmatch(line); len(m) > 1 {
			pkg := m[1]
			fmt.Fprintf(out, "[%03ds] ⏭  SKIP: %s (no test files)\n", elapsedSec, pkg)
			summary.Skipped++
		} else if m := rePass.FindStringSubmatch(line); len(m) > 2 {
			pkg := m[1]
			dur := m[2]
			fmt.Fprintf(out, "[%03ds] ✅ PASS: %s (%ss)\n", elapsedSec, pkg, dur)
			summary.Passed++
		} else if m := reFail.FindStringSubmatch(line); len(m) > 1 {
			pkg := m[1]
			fmt.Fprintf(out, "[%03ds] ❌ FAIL: %s\n", elapsedSec, pkg)
			summary.Failed++
		} else if strings.HasPrefix(line, "--- FAIL:") {
			fmt.Fprintf(out, "  %s\n", line)
		}
	}

	runErr := cmd.Wait()
	summary.Duration = time.Since(startTime)
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			summary.ExitCode = exitErr.ExitCode()
		} else {
			summary.ExitCode = 1
		}
	}

	fmt.Fprintf(out, "\n=========================================\n")
	fmt.Fprintf(out, "Test Execution Summary (%v)\n", summary.Duration.Round(time.Millisecond))
	fmt.Fprintf(out, "=========================================\n")
	fmt.Fprintf(out, "Passed:  %d\n", summary.Passed)
	fmt.Fprintf(out, "Failed:  %d\n", summary.Failed)
	fmt.Fprintf(out, "Skipped: %d\n", summary.Skipped)
	fmt.Fprintf(out, "Log:     %s\n", logFile)

	return summary, runErr
}
