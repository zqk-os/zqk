package git

import (
	"context"
	"fmt"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/execwrap"
)

// CloneOptions configures git clone operations.
type CloneOptions struct {
	Depth        int
	Branch       string
	SingleBranch bool
	Quiet        bool
}

// Clone clones a git repository from cloneURL into destDir using execwrap.
func Clone(ctx context.Context, cloneURL, destDir string, opts ...CloneOptions) error {
	args := []string{"clone"}

	var opt CloneOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	if opt.Depth > 0 {
		args = append(args, "--depth", fmt.Sprintf("%d", opt.Depth))
	}
	if opt.SingleBranch {
		args = append(args, "--single-branch")
	}
	if opt.Branch != "" {
		args = append(args, "--branch", opt.Branch)
	}
	if opt.Quiet {
		args = append(args, "--quiet")
	}

	args = append(args, cloneURL, destDir)

	cmd := execwrap.CommandContext(ctx, gitCommandName, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errfmt.Errorf("git clone failed (%s): %s", err, string(out))
	}
	return nil
}

// CloneShallow clones a git repository with depth 1.
func CloneShallow(ctx context.Context, cloneURL, destDir string) error {
	return Clone(ctx, cloneURL, destDir, CloneOptions{Depth: 1})
}

// CloneToFacade clones a repository and returns a Facade pointing to the cloned repository.
func CloneToFacade(ctx context.Context, cloneURL, destDir string, opts ...CloneOptions) (*Facade, error) {
	if err := Clone(ctx, cloneURL, destDir, opts...); err != nil {
		return nil, err
	}
	return NewFacade(destDir), nil
}
