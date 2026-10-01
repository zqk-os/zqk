package resourcehygiene

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/zqk-os/zqk/pkg/operational"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/utils/syscallutil"
)

// DefaultLockStaleAge is the default age at which a lock file without a holder is considered stale.
const DefaultLockStaleAge = 15 * time.Minute

// DefaultTempOrphanAge is the default age at which a temporary file is considered orphaned.
const DefaultTempOrphanAge = 30 * time.Minute

// IsTempFileName reports whether the base name looks like a temporary artifact.
func IsTempFileName(name string) bool {
	return strings.HasSuffix(name, ".tmp") ||
		strings.Contains(name, ".tmp-") ||
		strings.Contains(name, ".tmp.") ||
		strings.HasPrefix(name, ".tmp-") ||
		strings.HasSuffix(name, ".yaml.tmp") ||
		strings.HasPrefix(name, "pclimits-")
}

// IsLockFileName reports whether the base name looks like a lock file.
func IsLockFileName(name string) bool {
	return strings.HasSuffix(name, ".lock")
}

// InspectIOResources collects point-in-time I/O metrics, open file descriptors,
// volume capacity, directory storage breakdown, and hygiene anomalies.
func InspectIOResources(ctx context.Context, projectRoot string) (*IOResourceTelemetry, error) {
	now := time.Now().UTC()
	report := &IOResourceTelemetry{
		CapturedAt:        now.Format(time.RFC3339),
		ZqkChildren:       make(map[string]ChildFolderUsage),
		StaleLockPaths:    []string{},
		OrphanedTempPaths: []string{},
		OrphanedProcesses: []string{},
	}

	openFDs, maxFDs, _ := GetProcessFDUsage()
	report.OpenFileDescriptors = openFDs
	report.MaxFileDescriptors = maxFDs

	if projectRoot == "" {
		return report, nil
	}

	snap, _ := operational.RunFilesystemProjectSnapshot(ctx, projectRoot, operational.FSSnapshotScopeZqk)
	if snap != nil {
		report.TotalZqkFiles = snap.TotalFiles
		report.TotalZqkBytes = snap.TotalBytes
		if snap.Volume != nil {
			report.VolumeTotalBytes = snap.Volume.TotalBytes
			report.VolumeAvailableBytes = snap.Volume.AvailableBytes
		}
		for child, st := range snap.ZqkByChild {
			if st != nil {
				report.ZqkChildren[child] = ChildFolderUsage{
					Files: st.Files,
					Bytes: st.Bytes,
				}
			}
		}
	}

	zqkDir := filepath.Join(projectRoot, paths.ProjectDataDir)
	lockCutoff := now.Add(-DefaultLockStaleAge)
	tempCutoff := now.Add(-DefaultTempOrphanAge)

	_ = filepath.WalkDir(zqkDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if IsLockFileName(name) {
			if strings.Contains(filepath.ToSlash(path), "/state/daemon_locks/") {
				return nil
			}
			if info, errStat := d.Info(); errStat == nil {
				if info.ModTime().Before(lockCutoff) {
					// Verify whether the lock is held by a live process before declaring it stale
					file, errOpen := fileutil.OpenFile(path, fileutil.O_RDWR, paths.FilePerm600)
					if errOpen == nil {
						flockErr := syscallutil.FileFlock(file, syscall.LOCK_EX|syscall.LOCK_NB)
						if flockErr != nil {
							// Actively held by a live process: not stale!
							_ = file.Close()
							return nil
						}
						_ = syscallutil.FileFlock(file, syscall.LOCK_UN)
						_ = file.Close()
					}
					rel, relErr := filepath.Rel(projectRoot, path)
					if relErr == nil {
						report.StaleLockPaths = append(report.StaleLockPaths, rel)
					} else {
						report.StaleLockPaths = append(report.StaleLockPaths, path)
					}
				}
			}
		} else if IsTempFileName(name) {
			if info, errStat := d.Info(); errStat == nil {
				if info.ModTime().Before(tempCutoff) {
					rel, relErr := filepath.Rel(projectRoot, path)
					if relErr == nil {
						report.OrphanedTempPaths = append(report.OrphanedTempPaths, rel)
					} else {
						report.OrphanedTempPaths = append(report.OrphanedTempPaths, path)
					}
				}
			}
		}
		return nil
	})

	report.StaleLocksCount = len(report.StaleLockPaths)
	report.OrphanedTempCount = len(report.OrphanedTempPaths)

	cnt, orphanProcs, _ := ReapOrphanedProcesses(projectRoot, true)
	report.OrphanedProcessCount = cnt
	if len(orphanProcs) > 0 {
		report.OrphanedProcesses = orphanProcs
	}

	return report, nil
}
