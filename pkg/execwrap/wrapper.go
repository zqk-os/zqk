package execwrap

import (
	"bytes"
	"context"
	"os/exec"
)

func CommandContext(ctx context.Context, name string, arg ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, arg...) //nolint:gosec
}

func Command(name string, arg ...string) *exec.Cmd {
	return exec.Command(name, arg...) //nolint:gosec
}

// RunWithBuffers captures stdout and stderr from cmd into strings while executing it.
func RunWithBuffers(cmd *exec.Cmd) (stdout, stderr string, err error) {
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return outBuf.String(), errBuf.String(), err
}
