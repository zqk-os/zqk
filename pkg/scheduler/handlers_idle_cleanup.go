package scheduler

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/execwrap"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/maintenance"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// Stale agent worktrees under .zqk/worktrees are wiped even if the ATK still looks "active"
// (swarm spawn without teardown is the common failure mode).
const agentWorktreeStaleAfter = 6 * time.Hour

// Non-.zqk/worktrees git worktrees (legacy paths) still use a longer age gate.
const legacyWorktreeStaleAfter = 24 * time.Hour

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

	gitSvc := maintenance.NewGitMaintenanceService(h.projectRoot)
	removed := h.pruneAgentWorktreeDir(ctx, gitSvc, slog)
	removed += h.pruneLegacyGitWorktrees(ctx, slog)

	// Terminate idle agents and stalled background builds (pid stamp files).
	files, err := fileutil.ReadDir(filepath.Join(h.projectRoot, ".zqk", "run"))
	if err == nil {
		for _, f := range files {
			if strings.HasSuffix(f.Name(), ".pid") {
				pidPath := filepath.Join(h.projectRoot, ".zqk", "run", f.Name())
				if stat, err := fileutil.Stat(pidPath); err == nil {
					if time.Since(stat.ModTime()) > legacyWorktreeStaleAfter {
						slog.Info("Terminating stalled process from pid file").String("pid_file", pidPath).Log()
						_ = fileutil.Remove(pidPath)
					}
				}
			}
		}
	}

	slog.Info("idle_cleanup_complete").JobID(job.ID).Int("worktrees_removed", removed).Log()
	return nil
}

// pruneAgentWorktreeDir removes .zqk/worktrees/<ATK-*> when the task is missing,
// terminal, or the directory is older than agentWorktreeStaleAfter.
func (h *IdleCleanupHandler) pruneAgentWorktreeDir(ctx context.Context, gitSvc *maintenance.GitMaintenanceService, slog *SchedulerLogRoot) int {
	removed := 0
	for _, dir := range []string{
		paths.AgentWorktreeContainer(h.projectRoot),
		filepath.Join(h.projectRoot, ".zqk", "worktrees"),
	} {
		removed += h.pruneAgentWorktreeEntries(ctx, gitSvc, slog, dir)
	}
	return removed
}

func (h *IdleCleanupHandler) pruneAgentWorktreeEntries(ctx context.Context, gitSvc *maintenance.GitMaintenanceService, slog *SchedulerLogRoot, dir string) int {
	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		return 0
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	checker := objects.GetGlobalStatusChecker()
	removed := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := e.Name()
		path := filepath.Join(dir, id)
		reason, drop := h.shouldDropAgentWorktree(ctx, secCtx, checker, id, path)
		if !drop {
			continue
		}
		slog.Info("Removing agent worktree").Path(path).String("reason", reason).Log()
		if err := gitSvc.CleanupWorktreeAndBranchForID(ctx, id); err != nil {
			slog.Warn("CleanupWorktreeAndBranchForID failed; forcing remove").Path(path).WithError(err).Log()
			_ = runGit(ctx, h.projectRoot, "worktree", "remove", "-f", path)
			if err := paths.MustNotDestroyProjectRoot(h.projectRoot, path); err != nil {
				slog.Warn("Skipping os.RemoveAll due to wipe hazard check").Path(path).WithError(err).Log()
			} else {
				_ = fileutil.RemoveAll(path)
			}
			_ = runGit(ctx, h.projectRoot, "branch", "-D", "agent/"+id)
		}
		removed++
	}
	_ = runGit(ctx, h.projectRoot, "worktree", "prune")
	return removed
}

func (h *IdleCleanupHandler) shouldDropAgentWorktree(
	ctx context.Context,
	secCtx *storagepkg.SecurityContext,
	checker objects.IStatusChecker,
	id, path string,
) (reason string, drop bool) {
	stat, err := fileutil.Stat(path)
	age := time.Duration(0)
	if err == nil {
		age = time.Since(stat.ModTime())
	}

	if h.storage == nil {
		if age > agentWorktreeStaleAfter {
			return "no_storage_stale_mtime", true
		}
		return "", false
	}

	obj, readErr := h.storage.Read(ctx, secCtx, id)
	if readErr != nil || obj == nil {
		return "object_missing", true
	}
	status, _ := obj[objects.FieldKeyStatus].(string)
	kind, _ := obj[objects.FieldKeyKind].(string)
	if kind == "" {
		kind = objects.KindAgentTask
	}
	if checker.IsTerminal(kind, status) || checker.IsArchive(kind, status) {
		return "terminal_status:" + status, true
	}
	if age > agentWorktreeStaleAfter {
		// TRACK: REDACTED — tighten once swarm teardown is reliable end-to-end
		return "stale_active_mtime", true
	}
	return "", false
}

func (h *IdleCleanupHandler) pruneLegacyGitWorktrees(ctx context.Context, slog *SchedulerLogRoot) int {
	out, err := runGitOutput(ctx, h.projectRoot, "worktree", "list", "--porcelain")
	if err != nil {
		return 0
	}
	agentPrefix := filepath.Join(h.projectRoot, ".zqk", "worktrees") + string(fileutil.PathSeparator)
	isolatedPrefix := paths.AgentWorktreeContainer(h.projectRoot) + string(fileutil.PathSeparator)
	removed := 0
	var currentWT string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "worktree ") {
			currentWT = strings.TrimPrefix(line, "worktree ")
			continue
		}
		if !strings.HasPrefix(line, "branch refs/heads/") {
			continue
		}
		branch := strings.TrimPrefix(line, "branch refs/heads/")
		if currentWT == "" || branch == "main" || branch == "master" {
			continue
		}
		// Agent worktrees handled in pruneAgentWorktreeDir.
		if strings.HasPrefix(currentWT, agentPrefix) || strings.HasPrefix(currentWT, isolatedPrefix) {
			continue
		}
		stat, err := fileutil.Stat(currentWT)
		if err != nil || time.Since(stat.ModTime()) <= legacyWorktreeStaleAfter {
			continue
		}
		slog.Info("Removing orphaned legacy worktree").Path(currentWT).Log()
		_ = runGit(ctx, h.projectRoot, "worktree", "remove", "-f", currentWT)
		if err := paths.MustNotDestroyProjectRoot(h.projectRoot, currentWT); err != nil {
			slog.Warn("Skipping worktree cleanup due to hazard check").Path(currentWT).WithError(err).Log()
		} else {
			_ = fileutil.RemoveAll(currentWT)
		}
		removed++
	}
	return removed
}

func runGitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := execwrap.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}

func runGit(ctx context.Context, dir string, args ...string) error {
	cmd := execwrap.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	return cmd.Run()
}
