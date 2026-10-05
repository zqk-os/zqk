package scheduler

import (
	"path/filepath"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// rollupStatusCoreEventsMu serializes appends to rollup_status_core.jsonl (convergence CLI rollup runs).
var rollupStatusCoreEventsMu sync.Mutex

// RollupStatusCoreEventsFilePath returns the append-only JSONL path for rollup_status_core emissions:
// .zqk/logs/scheduler/rollup_status_core.jsonl
func RollupStatusCoreEventsFilePath(projectRoot string) string {
	return filepath.Join(JobLogsBaseDir(projectRoot), "rollup_status_core.jsonl")
}

// AppendRollupStatusCoreEvent appends one JSON line with timestamp, optional session id, and the
// rollup_status_core map (same shape as json/yaml output). Best-effort; errors are ignored.
func AppendRollupStatusCoreEvent(projectRoot, convergenceSessionID string, rollupStatusCore map[string]any) {
	if projectRoot == emptyValue || rollupStatusCore == nil {
		return
	}
	entry := map[string]any{
		"timestamp_rfc3339":    time.Now().UTC().Format(time.RFC3339),
		"rollup_status_core":   rollupStatusCore,
		objects.FieldKeySource: "zqk_scheduler_test_failures_convergence",
	}
	if convergenceSessionID != emptyValue {
		entry["convergence_session_id"] = convergenceSessionID
	}
	path := RollupStatusCoreEventsFilePath(projectRoot)
	rollupStatusCoreEventsMu.Lock()
	defer rollupStatusCoreEventsMu.Unlock()
	_ = fileutil.AppendJSONLine(path, entry)
}
