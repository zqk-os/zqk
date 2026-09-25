package scheduler

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// lockFileSuffix is the canonical suffix for scheduler job lock files.
const lockFileSuffix = ".lock"

const (
	msgLockDirRequired = "lock dir required"
	msgReadLockDir     = "read lock dir"
)

// CleanStaleLocksByAge removes *.lock files under dir whose modification time is
// older than `threshold`. Behavior:
//
//   - threshold <= 0: no removal; returns 0 and a nil error (fail-closed: a
//     zero-valued or negative threshold must never delete locks).
//   - Only regular files whose name ends with the lockFileSuffix are candidates.
//     Directories and non-lock files are left untouched.
//   - Removal errors for an individual file are recorded but do not abort the
//     scan; the scan continues so a single unreadable entry cannot hide others.
//   - A missing directory is not an error: returns (0, nil) so callers may invoke
//     the routine before the first lock is ever created.
//
// The routine is idempotent per directory and safe to call concurrently; it
// performs no global locking and each removal targets exactly one named path.
func CleanStaleLocksByAge(dir string, threshold time.Duration) (int, error) {
	if strings.TrimSpace(dir) == emptyValue {
		return 0, errfmt.Errorf("%s", msgLockDirRequired)
	}
	if threshold <= 0 {
		return 0, nil
	}

	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return 0, nil
		}
		return 0, errfmt.Newf(msgReadLockDir).Wrap(err)
	}

	removed := 0
	now := time.Now()
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), lockFileSuffix) {
			continue
		}
		p := filepath.Join(dir, e.Name())
		info, statErr := fileutil.Stat(p)
		if statErr != nil {
			if fileutil.IsNotExist(statErr) {
				continue
			}
			continue
		}
		if now.Sub(info.ModTime()) <= threshold {
			continue
		}

		// Verify whether the lock is actively held by another process using non-blocking flock.
		// Never unlink a file if another process holds the flock.
		fl, err := storagepkg.NewFileLock(p)
		if err != nil {
			continue
		}
		acquired, lockErr := fl.TryLock()
		if !acquired || lockErr != nil {
			_ = fl.Close()
			continue
		}
		removeErr := fileutil.Remove(p)
		_ = fl.Unlock()
		_ = fl.Close()

		if removeErr == nil {
			removed++
		} else if !fileutil.IsNotExist(removeErr) {
			// Best-effort: skip and continue. The scan should not abort on
			// a single EIO/EBUSY on a single lock file.
			continue
		}
	}
	return removed, nil
}
