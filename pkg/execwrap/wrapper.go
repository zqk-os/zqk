package execwrap

import (
	"context"
	"os/exec"
)

func CommandContext(ctx context.Context, name string, arg ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, arg...)
}

func Command(name string, arg ...string) *exec.Cmd {
	return exec.Command(name, arg...)
}
