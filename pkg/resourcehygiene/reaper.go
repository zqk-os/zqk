package resourcehygiene

import (
	"io/fs"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/utils/syscallutil"
)

// ReapOrphanedTempFiles traverses .zqk looking for temp files older than threshold,
// safely removing them.
func ReapOrphanedTempFiles(projectRoot string, threshold time.Duration, dryRun bool) (int, int64, []string, error) {
	if projectRoot == "" {
		return 0, 0, nil, nil
	}
	if threshold <= 0 {
		threshold = DefaultTempOrphanAge
	}

	zqkDir := filepath.Join(projectRoot, paths.ProjectDataDir)
	cutoff := time.Now().UTC().Add(-threshold)

	var count int
	var reclaimedBytes int64
	var reaped []string

	err := filepath.WalkDir(zqkDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if !IsTempFileName(name) {
			return nil
		}
		info, errStat := d.Info()
		if errStat != nil {
			return nil
		}
		if info.ModTime().Before(cutoff) {
			reclaimedBytes += info.Size()
			count++
			rel, relErr := filepath.Rel(projectRoot, path)
			if relErr == nil {
				reaped = append(reaped, rel)
			} else {
				reaped = append(reaped, path)
			}
			if !dryRun {
				_ = fileutil.Remove(path)
			}
		}
		return nil
	})

	return count, reclaimedBytes, reaped, err
}

// ReapStaleLocks scans .zqk for .lock files older than threshold and reaps them
// only if no active process holds an exclusive flock.
func ReapStaleLocks(projectRoot string, threshold time.Duration, dryRun bool) (int, []string, error) {
	if projectRoot == "" {
		return 0, nil, nil
	}
	if threshold <= 0 {
		threshold = DefaultLockStaleAge
	}

	zqkDir := filepath.Join(projectRoot, paths.ProjectDataDir)
	cutoff := time.Now().UTC().Add(-threshold)

	var count int
	var reaped []string

	err := filepath.WalkDir(zqkDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !IsLockFileName(d.Name()) {
			return nil
		}
		info, errStat := d.Info()
		if errStat != nil {
			return nil
		}
		if !info.ModTime().Before(cutoff) {
			return nil
		}

		// Try non-blocking exclusive lock to guarantee no live holder
		file, errOpen := fileutil.OpenFile(path, fileutil.O_RDWR, paths.FilePerm600)
		if errOpen != nil {
			// If file cannot be opened (e.g. permission/disappeared), skip
			return nil
		}

		flockErr := syscallutil.FileFlock(file, syscall.LOCK_EX|syscall.LOCK_NB)
		if flockErr != nil {
			// Held by a live process: do not touch
			_ = file.Close()
			return nil
		}

		// Lock acquired: safe to reap
		_ = syscallutil.FileFlock(file, syscall.LOCK_UN)
		_ = file.Close()

		count++
		rel, relErr := filepath.Rel(projectRoot, path)
		if relErr == nil {
			reaped = append(reaped, rel)
		} else {
			reaped = append(reaped, path)
		}

		if !dryRun {
			_ = fileutil.Remove(path)
		}

		return nil
	})

	return count, reaped, err
}

// isLogFile reports whether path/name is a candidate log file for retention enforcement.
func isLogFile(path, name string) bool {
	if name == "issues.json" || strings.HasSuffix(name, "-summary.json") || strings.HasSuffix(name, ".pid") || strings.HasSuffix(name, ".keepalive") {
		return false
	}
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".log", ".jsonl", ".stdout", ".stderr", ".stdio", ".out", ".err":
		return true
	case ".json", ".txt":
		return strings.Contains(path, "logs") || strings.Contains(path, "diagnostics") || strings.Contains(path, "events") || strings.Contains(path, "reports")
	default:
		return strings.HasSuffix(name, ".stdout") ||
			strings.HasSuffix(name, ".stderr") ||
			strings.HasSuffix(name, ".stdio")
	}
}

// EnforceLogRetention prunes or truncates oversized/aged logs under .zqk/logs, .zqk/mcp/logs, and .zqk/scheduler.
func EnforceLogRetention(projectRoot string, maxAge time.Duration, maxSizeBytes int64, dryRun bool) (int, int64, []string, error) {
	if projectRoot == "" {
		return 0, 0, nil, nil
	}
	if maxAge <= 0 {
		maxAge = 48 * time.Hour
	}
	if maxSizeBytes <= 0 {
		maxSizeBytes = 10 * 1024 * 1024 // 10MB
	}

	logDirs := []string{
		filepath.Join(projectRoot, paths.ProjectDataDir, paths.LogsDir),
		filepath.Join(projectRoot, paths.ProjectDataDir, paths.MCPDir, paths.MCPLogsDir),
		filepath.Join(projectRoot, paths.ProjectDataDir, paths.SchedulerDir),
	}

	cutoff := time.Now().UTC().Add(-maxAge)
	var count int
	var reclaimedBytes int64
	var reaped []string

	for _, dir := range logDirs {
		if _, err := fileutil.Stat(dir); err != nil {
			continue
		}
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				name := d.Name()
				if name == "state" || name == "locks" || name == "triggers" || name == "hourglass" {
					return filepath.SkipDir
				}
				return nil
			}
			name := d.Name()
			if !isLogFile(path, name) {
				return nil
			}

			info, errStat := d.Info()
			if errStat != nil {
				return nil
			}

			// Condition 1: Older than maxAge -> Delete
			if info.ModTime().Before(cutoff) {
				count++
				reclaimedBytes += info.Size()
				rel, _ := filepath.Rel(projectRoot, path)
				reaped = append(reaped, rel+" (deleted: age)")
				if !dryRun {
					_ = fileutil.Remove(path)
				}
				return nil
			}

			// Condition 2: Larger than maxSizeBytes -> Truncate to recent lines
			if info.Size() > maxSizeBytes {
				data, errRead := fileutil.ReadFile(path)
				if errRead == nil && int64(len(data)) > maxSizeBytes {
					lines := strings.Split(string(data), "\n")
					var kept []string
					if len(lines) > 10000 {
						kept = lines[len(lines)-10000:]
					} else if len(lines) > 100 {
						kept = lines[len(lines)/2:]
					} else if len(lines) > 1 {
						kept = lines[len(lines)-1:]
					}
					if len(kept) > 0 {
						newContent := []byte(strings.Join(kept, "\n"))
						freed := info.Size() - int64(len(newContent))
						if freed > 0 {
							count++
							reclaimedBytes += freed
							rel, _ := filepath.Rel(projectRoot, path)
							reaped = append(reaped, rel+" (truncated: size)")
							if !dryRun {
								_ = fileutil.WriteFile(path, newContent, paths.FilePerm600)
							}
						}
					}
				}
			}

			return nil
		})
	}

	return count, reclaimedBytes, reaped, nil
}

// ExecuteHygiene performs full resource hygiene according to opts.
func ExecuteHygiene(projectRoot string, opts HygieneOptions) (*HygieneExecutionReport, error) {
	now := time.Now().UTC()
	report := &HygieneExecutionReport{
		Timestamp:   now.Format(time.RFC3339),
		DryRun:      opts.DryRun,
		ReapedPaths: []string{},
		Errors:      []string{},
	}

	if opts.ReapTemp {
		cnt, bytes, paths, err := ReapOrphanedTempFiles(projectRoot, opts.TempThreshold, opts.DryRun)
		report.TempReaped += cnt
		report.BytesReclaimed += bytes
		report.ReapedPaths = append(report.ReapedPaths, paths...)
		if err != nil {
			report.Errors = append(report.Errors, err.Error())
		}
	}

	if opts.ReapLocks {
		cnt, paths, err := ReapStaleLocks(projectRoot, opts.LockThreshold, opts.DryRun)
		report.LocksReaped += cnt
		report.ReapedPaths = append(report.ReapedPaths, paths...)
		if err != nil {
			report.Errors = append(report.Errors, err.Error())
		}
	}

	if opts.EnforceRetention {
		cnt, bytes, paths, err := EnforceLogRetention(projectRoot, opts.LogMaxAge, opts.LogMaxSize, opts.DryRun)
		report.LogsPruned += cnt
		report.BytesReclaimed += bytes
		report.ReapedPaths = append(report.ReapedPaths, paths...)
		if err != nil {
			report.Errors = append(report.Errors, err.Error())
		}
	}

	return report, nil
}
