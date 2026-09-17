package scheduler

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"time"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqktime"
)

// runningJobsSnapshotFile is the relative path under projectRoot for cross-process
// visibility of jobs currently executing in the scheduler daemon.
// CLI `scheduler activity` reads this when GetGlobalScheduler() is nil (out-of-process).
// TRACK: BLI-1785443942668406000-1ec5c811 — avoid false Executing:0 while test bundles run.
const runningJobsSnapshotFile = "running_jobs.json"

// RunningJobsSnapshot is the on-disk shape under .zqk/scheduler/state/.
type RunningJobsSnapshot struct {
	UpdatedAt string   `json:"updated_at"`
	JobIDs    []string `json:"job_ids"`
}

// RunningJobsSnapshotPath returns the absolute path of the running-jobs snapshot file.
func RunningJobsSnapshotPath(projectRoot string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, "scheduler", "state", runningJobsSnapshotFile)
}

// PersistRunningJobsSnapshot writes the current running job ID set for out-of-process readers.
func PersistRunningJobsSnapshot(projectRoot string, jobIDs []string) error {
	if projectRoot == "" {
		return nil
	}
	ids := append([]string(nil), jobIDs...)
	sort.Strings(ids)
	snap := RunningJobsSnapshot{
		UpdatedAt: zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		JobIDs:    ids,
	}
	data, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	path := RunningJobsSnapshotPath(projectRoot)
	if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
		return err
	}
	return fileutil.WriteSecureFile(path, data)
}

// LoadRunningJobsSnapshot returns job IDs the daemon last persisted as running.
// ok is false when the file is missing or unreadable (caller should fall back).
func LoadRunningJobsSnapshot(projectRoot string) (jobIDs []string, updatedAt time.Time, ok bool) {
	if projectRoot == "" {
		return nil, time.Time{}, false
	}
	data, err := fileutil.ReadFile(RunningJobsSnapshotPath(projectRoot))
	if err != nil {
		return nil, time.Time{}, false
	}
	var snap RunningJobsSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, snap.UpdatedAt); err == nil {
		updatedAt = t
	}
	return snap.JobIDs, updatedAt, true
}
