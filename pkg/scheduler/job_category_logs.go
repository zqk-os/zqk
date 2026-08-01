package scheduler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/utils/fileutil"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/zqktime"
)

// categoryLogsMu serializes appends to category index files so multiple jobs don't interleave lines.
var categoryLogsMu sync.Mutex

// jobCategoryLogsDir returns the base directory for category index logs:
// .zqk/logs/scheduler/by-category/
func jobCategoryLogsDir(projectRoot string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.LogsDir, paths.SchedulerJobLogsSubdir, "by-category")
}

// normalizeCategoryName converts a category to a safe file stem (lowercase, alphanumerics and dashes).
func normalizeCategoryName(category string) string {
	if category == emptyValue {
		return "uncategorized"
	}
	s := strings.TrimSpace(strings.ToLower(category))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		// Collapse any non-alphanumeric sequences to a single dash.
		if b.Len() == 0 || b.String()[b.Len()-1] == '-' {
			continue
		}
		b.WriteRune('-')
	}
	out := b.String()
	if out == emptyValue {
		return "uncategorized"
	}
	return out
}

// appendCategoryLogEntry appends a single JSONL entry to the category index file for the job's category.
// Path: .zqk/logs/scheduler/by-category/<normalized-category>.events.jsonl
// Best-effort: errors are ignored so scheduler execution is never affected.
func appendCategoryLogEntry(projectRoot string, job *ScheduledJob, eventType string, duration time.Duration, err error) {
	if projectRoot == emptyValue || job == nil {
		return
	}
	entry := map[string]any{
		"timestamp":                 zqktime.NowRFC3339UTC(),
		"job_id":                    job.ID,
		objects.FieldKeyJobType:     job.JobType,
		objects.FieldKeyCategory:    job.Category,
		objects.FieldKeyTriggerType: job.TriggerType,
		objects.FieldKeyEventType:   eventType,
	}
	if job.Title != emptyValue {
		entry[objects.FieldKeyTitle] = job.Title
	}
	if duration > 0 {
		entry["duration"] = duration.String()
	}
	if err != nil {
		entry["error"] = err.Error()
	}
	data, mErr := json.Marshal(entry)
	if mErr != nil {
		return
	}

	categoryName := normalizeCategoryName(job.Category)
	dir := jobCategoryLogsDir(projectRoot)
	path := filepath.Join(dir, categoryName+".events.jsonl")
	legacyPath := filepath.Join(dir, categoryName+".events.json")
	if err := migrateLegacyPath(legacyPath, path); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		SLog(logger).Debug("Failed to migrate legacy category log path").WithError(err).Log()
	}

	categoryLogsMu.Lock()
	defer categoryLogsMu.Unlock()

	if mkErr := fileutil.EnsureDir(dir); mkErr != nil {
		return
	}
	f, openErr := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if openErr != nil {
		return
	}
	if _, err := f.Write(data); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		SLog(logger).Debug("Failed to write category log data").WithError(err).Log()
	}
	if _, err := f.WriteString("\n"); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		SLog(logger).Debug("Failed to write category log newline").WithError(err).Log()
	}
	if err := f.Close(); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		SLog(logger).Debug("Failed to close category log file").WithError(err).Log()
	}
}
