package system

import (
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// CleanupQuarantineContext holds state for cleanup-quarantine.
type CleanupQuarantineContext struct {
	ProjectRoot    string
	QuarantineRoot string
	OlderThan      time.Duration
	DryRun         bool
	Verbose        bool
	Logger         logging.Logger
}

func initializeCleanupQuarantineContext(cmd *cobra.Command) (*CleanupQuarantineContext, error) {
	projectRoot := ProjectRootOrResolve("")
	if projectRoot == emptyValue {
		return nil, errfmt.Errorf("project root not found")
	}
	quarantineRoot := filepath.Join(projectRoot, paths.ProjectDataDir, paths.SystemHealthDir, paths.QuarantineDir)
	olderThan, _ := cmd.Flags().GetDuration("older-than")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	verbose, _ := cmd.Flags().GetBool("verbose")
	return &CleanupQuarantineContext{
		ProjectRoot:    projectRoot,
		QuarantineRoot: quarantineRoot,
		OlderThan:      olderThan,
		DryRun:         dryRun,
		Verbose:        verbose,
		Logger:         logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}, nil
}

// RunCleanupQuarantine scans quarantine and removes (or reports) files older than ctx.OlderThan.
// Returns number of files removed (or that would be removed if dry-run) and any errors.
func RunCleanupQuarantine(ctx *CleanupQuarantineContext) (removed int, errs []error) {
	info, err := fileutil.Stat(ctx.QuarantineRoot)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return 0, nil
		}
		return 0, []error{err}
	}
	if !info.IsDir() {
		return 0, []error{errfmt.Errorf("quarantine path is not a directory: %s", ctx.QuarantineRoot)}
	}
	cutoff := time.Now().Add(-ctx.OlderThan)
	err = filepath.Walk(ctx.QuarantineRoot, func(path string, info fileutil.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // continue walking on error
		}
		if info.IsDir() || !fileutil.IsYAMLPath(path) {
			return nil
		}
		if info.ModTime().After(cutoff) {
			return nil
		}
		if ctx.DryRun {
			if ctx.Verbose {
				logging.Fluent(ctx.Logger).Info("Would remove (older than threshold)").
					Path(path).
					String("mod_time", info.ModTime().Format(time.RFC3339)).
					Log()
			}
			removed++
			return nil
		}
		if err := fileutil.Remove(path); err != nil {
			errs = append(errs, errfmt.Errorf("remove %s: %w", path, err))
			return nil
		}
		removed++
		if ctx.Verbose {
			logging.Fluent(ctx.Logger).Debug("Removed quarantined file").Path(path).Log()
		}
		return nil
	})
	if err != nil {
		errs = append(errs, err)
	}
	return removed, errs
}
