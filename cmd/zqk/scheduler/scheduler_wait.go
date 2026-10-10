package scheduler

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/lifecycle"
)

const (
	flagTimeout              = "timeout"
	flagPollInterval         = "poll-interval"
	defaultWaitTimeout       = 5 * time.Minute
	defaultWaitPollInterval  = 50 * time.Millisecond
	defaultJobStatusComplete = "completed"
)

var (
	// ErrWaitTimeout is returned when waiting for job completion exceeds the configured timeout.
	ErrWaitTimeout = errfmt.Errorf("scheduler wait: timed out waiting for job completion")
	// ErrJobNotFound is returned when an invalid or empty job ID is provided.
	ErrJobNotFound = errfmt.Errorf("scheduler wait: job ID is required")
)

// WaitResult encapsulates the completion information for an awaited scheduler job.
type WaitResult struct {
	JobID       string            `json:"job_id"`
	Status      string            `json:"status"`
	Duration    string            `json:"duration"`
	DurationMs  int64             `json:"duration_ms"`
	Scope       map[string]string `json:"scope,omitempty"`
	CompletedAt time.Time         `json:"completed_at"`
}

// NewWaitCmd creates the scheduler wait command.
func NewWaitCmd() *cobra.Command {
	waitCmd := bldr_cli_cmd_v1.NewSchedulerWaitCommandBuilder()
	waitCmd.Args = cobra.ExactArgs(1)
	cli.BindAsyncProgress(waitCmd, runWait)
	cli.AddCommonFlags(waitCmd)

	return waitCmd
}

func runWait(cmd *cobra.Command, args []string) error {
	if len(args) == 0 || strings.TrimSpace(args[0]) == emptyValue {
		return ErrJobNotFound
	}
	jobID := strings.TrimSpace(args[0])

	timeout, err := cmd.Flags().GetDuration(flagTimeout)
	if err != nil || timeout <= 0 {
		timeout = defaultWaitTimeout
	}

	pollInterval, err := cmd.Flags().GetDuration(flagPollInterval)
	if err != nil || pollInterval <= 0 {
		pollInterval = defaultWaitPollInterval
	}

	projectRoot, err := resolveSchedulerProjectRoot(cmd)
	if err != nil {
		return err
	}

	parentCtx := cmd.Context()
	if parentCtx == nil {
		parentCtx = context.Background()
	}

	waitCtx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()

	start := time.Now()
	ev, err := lifecycle.AwaitJobWithWAL(waitCtx, projectRoot, jobID, pollInterval)
	if err != nil {
		if waitCtx.Err() == context.DeadlineExceeded {
			return errfmt.Errorf("%w: %s (after %s)", ErrWaitTimeout, jobID, timeout)
		}
		if waitCtx.Err() == context.Canceled {
			return errfmt.Errorf("scheduler wait canceled for job %s: %w", jobID, waitCtx.Err())
		}
		return errfmt.Errorf("failed awaiting scheduler job %s: %w", jobID, err)
	}

	elapsed := time.Since(start)
	status := ev.ToStatus
	if status == emptyValue {
		status = defaultJobStatusComplete
	}

	res := &WaitResult{
		JobID:       jobID,
		Status:      status,
		Duration:    elapsed.Round(time.Millisecond).String(),
		DurationMs:  elapsed.Milliseconds(),
		Scope:       ev.Scope,
		CompletedAt: ev.Timestamp(),
	}

	return outputWaitResult(cmd, res)
}

func outputWaitResult(cmd *cobra.Command, res *WaitResult) error {
	format := cli.GetFormat(cmd)
	switch format {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return cli.FormatOutput(cmd, res)
	default:
		return outputWaitResultTable(cmd, res)
	}
}

func outputWaitResultTable(cmd *cobra.Command, res *WaitResult) error {
	var buf strings.Builder
	fmt.Fprintf(&buf, "Job ID:       %s\n", res.JobID)
	fmt.Fprintf(&buf, "Status:       %s\n", res.Status)
	fmt.Fprintf(&buf, "Duration:     %s\n", res.Duration)
	if !res.CompletedAt.IsZero() {
		fmt.Fprintf(&buf, "Completed At: %s\n", res.CompletedAt.Format(time.RFC3339))
	}
	if len(res.Scope) > 0 {
		buf.WriteString("Scope:\n")
		keys := make([]string, 0, len(res.Scope))
		for k := range res.Scope {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&buf, "  %s: %s\n", k, res.Scope[k])
		}
	}
	return cli.WriteOutput(cmd, []byte(buf.String()))
}
