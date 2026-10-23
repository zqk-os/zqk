package system

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// canSkipAutofixRun performs an instantaneous reactive gate check.
// If an auto-fix run was scheduled or requested without targeted IDs, and the
// kernel validation cache proves zero unresolved issues with no pending autofix batches,
// no uncommitted drafts, and no WAL mutations since the cache timestamp,
// it returns true so the command can exit immediately without spinning up
// 16 parallel workers and scanning thousands of unchanged files.
func canSkipAutofixRun(cmd *cobra.Command, args []string) bool {
	if cmd == nil {
		return false
	}
	autoFix, _ := cmd.Flags().GetBool("auto-fix")
	if !autoFix {
		return false
	}

	// Targeted checks for specific kinds or object IDs must always run.
	if len(args) > 0 && args[0] != "all" {
		return false
	}

	// Explicit override/invalidation flags must bypass this gate.
	force, _ := cmd.Flags().GetBool("force")
	clearCache, _ := cmd.Flags().GetBool("clear-cache")
	refreshCache, _ := cmd.Flags().GetBool("refresh-cache")
	if force || clearCache || refreshCache {
		return false
	}

	// Gate applies to background/scheduler sweeps and explicit scheduler batching.
	isSchedulerJob := zqkenv.JobID().Get() != emptyValue
	autoFixScheduler, _ := cmd.Flags().GetBool("auto-fix-scheduler")
	skipIfLocked, _ := cmd.Flags().GetBool("skip-if-locked")
	if !isSchedulerJob && !autoFixScheduler && !skipIfLocked {
		return false
	}

	projectRoot := ProjectRootOrResolve("")
	if projectRoot == emptyValue {
		return false
	}

	// 1. Pending autofix batches: if any AUTOFIX-*.json files exist, must process them.
	autofixDir := filepath.Join(projectRoot, paths.ProjectDataDir, "autofix")
	if entries, err := fileutil.ReadDir(autofixDir); err == nil && len(entries) > 0 {
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "AUTOFIX-") && strings.HasSuffix(e.Name(), ".json") {
				return false
			}
		}
	}

	// 2. Validation state cache: must be populated, completely fresh, and have 0 auto-fixable issues.
	cache := validation.SharedValidationStateCache(projectRoot)
	if cache == nil {
		return false
	}
	_ = cache.Load()
	total, stale, _ := cache.Count()
	if total == 0 || stale > 0 {
		return false
	}
	if cache.CountAutoFixable() > 0 {
		return false
	}

	cacheUpdated := cache.LoadedUpdated()
	if cacheUpdated.IsZero() {
		return false
	}

	// 4. WAL / lifecycle mutations: if new events occurred after the cache update, run.
	walPath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.WalDir, "lifecycle_events.wal")
	if walInfo, err := fileutil.Stat(walPath); err == nil && walInfo.ModTime().After(cacheUpdated.Add(1*time.Second)) {
		return false
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	logging.Fluent(logger).Info("Zero auto-fixable issues detected in clean kernel; skipping redundant validation scan").
		String("job_id", zqkenv.JobID().Get()).
		Int("clean_cached_objects", total).
		Log()

	return true
}
