package scheduler

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/maintenance"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
)

type IdleCleanupHandler struct {
	projectRoot string
	logger      logging.Logger
	storage     storagepkg.ObjectStorageProvider
}

func NewIdleCleanupHandler(projectRoot string, logger logging.Logger, storage storagepkg.ObjectStorageProvider) *IdleCleanupHandler {
	return &IdleCleanupHandler{
		projectRoot: projectRoot,
		logger:      logger,
		storage:     storage,
	}
}

func (h *IdleCleanupHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	slog := SLog(h.logger)
	slog.Info("idle_cleanup_start").JobID(job.ID).Log()

	// 1. Terminate orphaned worktrees
	gitSvc := maintenance.NewGitMaintenanceService(h.projectRoot)
	_ = gitSvc
	out, err := runGitOutput(ctx, h.projectRoot, "worktree", "list", "--porcelain")
	if err == nil {
		var currentWT string
		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(line, "worktree ") {
				currentWT = strings.TrimPrefix(line, "worktree ")
			} else if strings.HasPrefix(line, "branch refs/heads/") {
				branch := strings.TrimPrefix(line, "branch refs/heads/")
				if currentWT != "" && branch != "main" && branch != "master" {
					if stat, err := os.Stat(currentWT); err == nil {
						if time.Since(stat.ModTime()) > 24*time.Hour {
							slog.Info("Removing orphaned worktree").String("path", currentWT).Log()
							_ = runGit(ctx, h.projectRoot, "worktree", "remove", "-f", currentWT)
							_ = os.RemoveAll(currentWT)
						}
					}
				}
			}
		}
	}

	// 2. Terminate idle agents and stalled background builds
	files, err := os.ReadDir(filepath.Join(h.projectRoot, ".zqk", "run"))
	if err == nil {
		for _, f := range files {
			if strings.HasSuffix(f.Name(), ".pid") {
				pidPath := filepath.Join(h.projectRoot, ".zqk", "run", f.Name())
				if stat, err := os.Stat(pidPath); err == nil {
					if time.Since(stat.ModTime()) > 24*time.Hour {
						slog.Info("Terminating stalled process from pid file").String("pid_file", pidPath).Log()
						_ = os.Remove(pidPath)
					}
				}
			}
		}
	}

	slog.Info("idle_cleanup_complete").JobID(job.ID).Log()
	return nil
}

func runGitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}

func runGit(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	return cmd.Run()
}
