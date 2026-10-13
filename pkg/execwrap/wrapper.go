package execwrap

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

func CommandContext(ctx context.Context, name string, arg ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, arg...) //nolint:gosec
}

func Command(name string, arg ...string) *exec.Cmd {
	return exec.Command(name, arg...) //nolint:gosec
}

// CommandContextWithGroup creates an *exec.Cmd configured in an isolated process group.
// When ctx cancels or times out, cmd.Cancel is triggered to terminate the entire process group.
func CommandContextWithGroup(ctx context.Context, name string, arg ...string) *exec.Cmd {
	cmd := CommandContext(ctx, name, arg...)
	SetProcessGroup(cmd)
	cmd.Cancel = func() error {
		return KillProcessGroup(cmd)
	}
	cmd.WaitDelay = 2 * time.Second
	return cmd
}

// CommandWithTimeout returns an *exec.Cmd bound to a timeout context,
// along with a cancel function that must be deferred or called by the caller.
func CommandWithTimeout(ctx context.Context, timeout time.Duration, name string, arg ...string) (*exec.Cmd, context.CancelFunc) {
	tctx, cancel := context.WithTimeout(ctx, timeout)
	cmd := CommandContext(tctx, name, arg...)
	return cmd, cancel
}

// RunWithTimeout executes a command with a bounded timeout duration.
// If the command exceeds the timeout, it terminates the process, returns
// accumulated stdout/stderr buffers, and sets timedOut to true with context.DeadlineExceeded.
func RunWithTimeout(ctx context.Context, timeout time.Duration, name string, arg ...string) (stdout, stderr string, timedOut bool, err error) {
	if timeout <= 0 {
		cmd := CommandContext(ctx, name, arg...)
		stdout, stderr, err = RunWithBuffers(cmd)
		return stdout, stderr, false, err
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := CommandContext(timeoutCtx, name, arg...)
	return executeWithTimeoutContext(timeoutCtx, cmd)
}

// RunWithTimeoutGroup executes a command within an isolated process group and a bounded timeout duration.
// If the command exceeds the timeout, the entire process group is terminated, returning
// accumulated stdout/stderr buffers, and sets timedOut to true with context.DeadlineExceeded.
func RunWithTimeoutGroup(ctx context.Context, timeout time.Duration, name string, arg ...string) (stdout, stderr string, timedOut bool, err error) {
	if timeout <= 0 {
		cmd := CommandContextWithGroup(ctx, name, arg...)
		stdout, stderr, err = RunWithBuffers(cmd)
		return stdout, stderr, false, err
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := CommandContextWithGroup(timeoutCtx, name, arg...)
	return executeWithTimeoutContext(timeoutCtx, cmd)
}

func executeWithTimeoutContext(timeoutCtx context.Context, cmd *exec.Cmd) (stdout, stderr string, timedOut bool, err error) {
	stdout, stderr, runErr := RunWithBuffers(cmd)
	if timeoutCtx.Err() == context.DeadlineExceeded {
		return stdout, stderr, true, context.DeadlineExceeded
	}
	return stdout, stderr, false, runErr
}

// RunWithBuffers captures stdout and stderr from cmd into strings while executing it.
func RunWithBuffers(cmd *exec.Cmd) (stdout, stderr string, err error) {
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

// RunWithStandardStreams connects os.Stdout and os.Stderr to cmd and executes it.
func RunWithStandardStreams(cmd *exec.Cmd) error {
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// IsTextFileBusy reports whether err represents an ETXTBSY (text file busy) execution error.
func IsTextFileBusy(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.ETXTBSY) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "text file busy")
}

// IsETXTBSY is an alias for IsTextFileBusy.
func IsETXTBSY(err error) bool {
	return IsTextFileBusy(err)
}

// RunCombinedOutputWithRetry executes a command, retrying with exponential backoff
// if the OS returns ETXTBSY (text file busy). This eliminates transient flakes on Linux
// when executing freshly written binaries or test mock scripts.
func RunCombinedOutputWithRetry(ctx context.Context, cmdFactory func() *exec.Cmd, maxRetries int, initialBackoff time.Duration) ([]byte, error) {
	if maxRetries <= 0 {
		maxRetries = 5
	}
	if initialBackoff <= 0 {
		initialBackoff = 10 * time.Millisecond
	}
	backoff := initialBackoff
	for attempt := 0; attempt <= maxRetries; attempt++ {
		cmd := cmdFactory()
		out, err := cmd.CombinedOutput()
		if err == nil {
			return out, nil
		}
		if !IsTextFileBusy(err) || attempt == maxRetries {
			return out, err
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > 200*time.Millisecond {
			backoff = 200 * time.Millisecond
		}
	}
	return nil, nil
}
