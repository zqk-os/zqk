package resourcehygiene

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/mitchellh/go-ps"
	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/daemon/singleton"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/process"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/utils/syscallutil"
)

type procEntry struct {
	pid  int
	ppid int
	exe  string
}

var (
	listProcessesHook = func() ([]procEntry, error) {
		procs, err := ps.Processes()
		if err != nil {
			return nil, err
		}
		res := make([]procEntry, len(procs))
		for i, p := range procs {
			res[i] = procEntry{pid: p.Pid(), ppid: p.PPid(), exe: p.Executable()}
		}
		return res, nil
	}
	getCommandLineHook = func(pid int) (string, error) {
		return process.ProcessCommandLine(pid), nil
	}
	killProcessHook = func(pid int, sig syscall.Signal) error {
		if runtime.GOOS == "windows" {
			p, err := os.FindProcess(pid)
			if err == nil {
				return p.Kill()
			}
			return err
		}
		_ = syscall.Kill(-pid, sig)
		return syscall.Kill(pid, sig)
	}
	isProcessAliveHook = func(pid int) bool {
		if runtime.GOOS == "windows" {
			p, err := os.FindProcess(pid)
			return err == nil && p != nil
		}
		return syscall.Kill(pid, 0) == nil
	}
)

// ReapOrphanedTempFiles traverses .zqk looking for temp files older than threshold,
type reaperTarget struct {
	zqkDir string
	cutoff time.Time
	valid  bool
}

func initReaperTarget(projectRoot string, threshold, defaultThreshold time.Duration) reaperTarget {
	if projectRoot == "" {
		return reaperTarget{}
	}
	if threshold <= 0 {
		threshold = defaultThreshold
	}
	return reaperTarget{
		zqkDir: filepath.Join(projectRoot, paths.ProjectDataDir),
		cutoff: time.Now().UTC().Add(-threshold),
		valid:  true,
	}
}

type reaperStats struct {
	count          int
	reclaimedBytes int64
	reaped         []string
}

// ReapOrphanedTempFiles scans .zqk for abandoned .tmp.* and .pending.* files older than threshold,
// safely removing them.
func ReapOrphanedTempFiles(projectRoot string, threshold time.Duration, dryRun bool) (int, int64, []string, error) {
	target := initReaperTarget(projectRoot, threshold, DefaultTempOrphanAge)
	if !target.valid {
		return 0, 0, nil, nil
	}

	var stats reaperStats

	err := filepath.WalkDir(target.zqkDir, func(path string, d fs.DirEntry, err error) error {
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
		if info.ModTime().Before(target.cutoff) {
			stats.reclaimedBytes += info.Size()
			stats.count++
			rel, relErr := filepath.Rel(projectRoot, path)
			if relErr == nil {
				stats.reaped = append(stats.reaped, rel)
			} else {
				stats.reaped = append(stats.reaped, path)
			}
			if !dryRun {
				_ = fileutil.Remove(path)
			}
		}
		return nil
	})

	return stats.count, stats.reclaimedBytes, stats.reaped, err
}

// ReapStaleLocks scans .zqk for .lock files older than threshold and reaps them
// only if no active process holds an exclusive flock.
func ReapStaleLocks(projectRoot string, threshold time.Duration, dryRun bool) (int, []string, error) {
	target := initReaperTarget(projectRoot, threshold, DefaultLockStaleAge)
	if !target.valid {
		return 0, nil, nil
	}

	var count int
	var reaped []string

	err := filepath.WalkDir(target.zqkDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !IsLockFileName(d.Name()) {
			return nil
		}
		if strings.Contains(filepath.ToSlash(path), "/state/daemon_locks/") {
			return nil
		}
		info, errStat := d.Info()
		if errStat != nil {
			return nil
		}
		if !info.ModTime().Before(target.cutoff) {
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
	target := initReaperTarget(projectRoot, maxAge, 48*time.Hour)
	if !target.valid {
		return 0, 0, nil, nil
	}
	if maxSizeBytes <= 0 {
		maxSizeBytes = 10 * 1024 * 1024 // 10MB
	}

	logDirs := []string{
		filepath.Join(target.zqkDir, paths.LogsDir),
		filepath.Join(target.zqkDir, paths.MCPDir, paths.MCPLogsDir),
		filepath.Join(target.zqkDir, paths.SchedulerDir),
	}

	var stats reaperStats

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
			if info.ModTime().Before(target.cutoff) {
				stats.count++
				stats.reclaimedBytes += info.Size()
				rel, _ := filepath.Rel(projectRoot, path)
				stats.reaped = append(stats.reaped, rel+" (deleted: age)")
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
							stats.count++
							stats.reclaimedBytes += freed
							rel, _ := filepath.Rel(projectRoot, path)
							stats.reaped = append(stats.reaped, rel+" (truncated: size)")
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

	return stats.count, stats.reclaimedBytes, stats.reaped, nil
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

	if opts.ReapProcesses {
		cnt, paths, err := ReapOrphanedProcesses(projectRoot, opts.DryRun)
		report.ProcessesReaped += cnt
		report.ReapedPaths = append(report.ReapedPaths, paths...)
		if err != nil {
			report.Errors = append(report.Errors, err.Error())
		}
	}

	return report, nil
}

// ReapOrphanedProcesses detects and terminates orphaned zqk processes whose parent has died (PPID == 1)
// that belong to projectRoot. Active daemons holding singleton locks, the Overseer, and the current
// process/ancestors are explicitly protected.
func ReapOrphanedProcesses(projectRoot string, dryRun bool) (int, []string, error) {
	if strings.TrimSpace(projectRoot) == "" {
		return 0, nil, nil
	}
	cleanRoot := filepath.Clean(projectRoot)

	// Fetch all actively held daemon locks under projectRoot so legitimate daemons are never touched
	activeDaemons, err := singleton.ActiveDaemonPIDs(cleanRoot)
	if err != nil {
		activeDaemons = make(map[string]int)
	}
	activePIDs := make(map[int]string, len(activeDaemons))
	for name, pid := range activeDaemons {
		activePIDs[pid] = name
	}

	procs, err := listProcessesHook()
	if err != nil {
		return 0, nil, errfmt.Newf("list system processes").Wrap(err)
	}

	selfPID := os.Getpid()
	var count int
	var reaped []string

	for _, p := range procs {
		pid := p.pid
		if pid <= 1 || pid == selfPID {
			continue
		}

		// Only processes whose parent has terminated and been reparented to 1 are orphans
		if p.ppid != 1 {
			continue
		}

		// Never touch current process ancestors
		if process.IsAncestorPID(pid, selfPID) {
			continue
		}

		// Never touch actively held daemons (overseer, steward, scheduler, etc.)
		if _, isActiveDaemon := activePIDs[pid]; isActiveDaemon {
			continue
		}

		// Executable name check: must be a branded or product executable
		base := filepath.Base(p.exe)
		if !brand.IsProductExecutable(base) && !strings.Contains(base, brand.ExecutableName()) && !strings.Contains(base, "zqk") {
			continue
		}

		// Verify that this process belongs to this project root via its command line
		cmdLine, errCmd := getCommandLineHook(pid)
		if errCmd != nil || strings.TrimSpace(cmdLine) == "" {
			continue
		}
		if !strings.Contains(cmdLine, cleanRoot) {
			continue
		}

		// Confirmed: orphaned zqk process parented by 1 belonging to this project root!
		label := fmt.Sprintf("PID %d: %s", pid, base)
		if dryRun {
			reaped = append(reaped, label)
			count++
			continue
		}

		// Live termination: attempt graceful SIGTERM first
		_ = killProcessHook(pid, syscall.SIGTERM)
		time.Sleep(150 * time.Millisecond)

		// If still alive, escalate to SIGKILL
		if isProcessAliveHook(pid) {
			_ = killProcessHook(pid, syscall.SIGKILL)
		}

		reaped = append(reaped, label+" (terminated)")
		count++
	}

	return count, reaped, nil
}
