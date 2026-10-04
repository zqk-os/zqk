package execwrap

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"time"
)

func CommandContext(ctx context.Context, name string, arg ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, arg...) //nolint:gosec
}

func Command(name string, arg ...string) *exec.Cmd {
	return exec.Command(name, arg...) //nolint:gosec
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
