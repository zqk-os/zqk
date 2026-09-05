package ambient

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

const (
	gitCmd = "git"
	ghCmd  = "gh"
	goCmd  = "go"
	zqkCmd = "./bin/zqk"
)

// PR represents a GitHub Pull Request payload from `gh pr list`.
type PR struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	HeadRefName string `json:"headRefName"`
	Mergeable   string `json:"mergeable"` // MERGEABLE, CONFLICTING, UNKNOWN
}

// AutoMergeDaemon continuously polls active pull requests, verifies test coverage,
// policy adherence, and automatically merges them if fully verified.
type AutoMergeDaemon struct {
	logger logging.Logger
	execFn func(ctx context.Context, name string, arg ...string) *exec.Cmd
}

// NewAutoMergeDaemon creates a new AutoMergeDaemon instance.
func NewAutoMergeDaemon(logger logging.Logger) *AutoMergeDaemon {
	return &AutoMergeDaemon{
		logger: logger,
		execFn: exec.CommandContext,
	}
}

// Start begins the polling loop for the auto-merge daemon.
func (d *AutoMergeDaemon) Start(ctx context.Context) {
	logging.Fluent(d.logger).Info("Starting Autonomy Inbox Auto-Merge Daemon...").Log()
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	// Initial poll on startup
	d.poll(ctx)

	for {
		select {
		case <-ctx.Done():
			logging.Fluent(d.logger).Info("Shutting down Auto-Merge Daemon.").Log()
			return
		case <-ticker.C:
			d.poll(ctx)
		}
	}
}

func (d *AutoMergeDaemon) poll(ctx context.Context) {
	cmd := d.execFn(ctx, ghCmd, "pr", "list", "--json", "number,title,headRefName,mergeable")
	out, err := cmd.Output()
	if err != nil {
		logging.Fluent(d.logger).Error("Failed to fetch PRs", errfmt.Errorf("gh command failed: %w", err)).Log()
		return
	}

	var prs []PR
	if err := json.Unmarshal(out, &prs); err != nil {
		logging.Fluent(d.logger).Error("Failed to parse PRs", errfmt.Errorf("unmarshal failed: %w", err)).Log()
		return
	}

	for _, pr := range prs {
		if pr.Mergeable != "MERGEABLE" {
			continue
		}
		logging.Fluent(d.logger).Info("Evaluating PR").WithFields(logging.PRNumberField(pr.Number), logging.String("pr_title", pr.Title)).Log()

		if err := d.verifyAndMerge(ctx, pr); err != nil {
			logging.Fluent(d.logger).Error("Failed to verify/merge PR", errfmt.Errorf("verify/merge failed: %w", err)).WithFields(logging.PRNumberField(pr.Number)).Log()
		}
	}
}

func (d *AutoMergeDaemon) verifyAndMerge(ctx context.Context, pr PR) error {
	// Create a temporary directory for the worktree
	tmpDir, err := fileutil.MkdirTemp("", "zqk-automerge-*")
	if err != nil {
		return errfmt.Newf("failed to create temp dir").Wrap(err)
	}
	defer fileutil.RemoveAll(tmpDir)

	// Fetch the branch
	if err := d.execFn(ctx, gitCmd, "fetch", "origin", pr.HeadRefName).Run(); err != nil {
		return errfmt.Newf("git fetch failed").Wrap(err)
	}

	// Create worktree
	if err := d.execFn(ctx, gitCmd, "worktree", "add", tmpDir, pr.HeadRefName).Run(); err != nil {
		return errfmt.Newf("git worktree add failed").Wrap(err)
	}
	defer func() {
		_ = d.execFn(context.Background(), gitCmd, "worktree", "remove", "--force", tmpDir).Run()
	}()

	// Build all in worktree
	cmdBuild := d.execFn(ctx, "make", "build-all")
	cmdBuild.Dir = tmpDir
	if err := cmdBuild.Run(); err != nil {
		return errfmt.Newf("make build-all failed in worktree").Wrap(err)
	}

	// Verify tests in worktree
	cmdVerify := d.execFn(ctx, "make", "verify")
	cmdVerify.Dir = tmpDir
	if err := cmdVerify.Run(); err != nil {
		return errfmt.Newf("make verify failed in worktree").Wrap(err)
	}

	// If fully verified, automatically merge
	if err := d.execFn(ctx, ghCmd, "pr", "merge", fmt.Sprintf("%d", pr.Number), "--auto", "--merge").Run(); err != nil {
		return errfmt.Newf("gh pr merge failed").Wrap(err)
	}

	logging.Fluent(d.logger).Info("Successfully verified and merged PR").WithFields(logging.PRNumberField(pr.Number)).Log()
	return nil
}
