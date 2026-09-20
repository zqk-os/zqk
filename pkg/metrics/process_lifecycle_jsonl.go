package metrics

import (
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// ProcessLifecycleEventCriteriaAutovalidateBatch aggregates one SCH-run-* autovalidate pass.
const ProcessLifecycleEventCriteriaAutovalidateBatch = "criteria_autovalidate_batch"

// ProcessLifecycleEventLifecycleTransitionApplied records a successful lifecycle-driven status update.
const ProcessLifecycleEventLifecycleTransitionApplied = "lifecycle_transition_applied"

// ProcessLifecycleEventLifecycleTransitionFailed records updater errors (validation, storage, etc.).
const ProcessLifecycleEventLifecycleTransitionFailed = "lifecycle_transition_failed"

// AppendProcessLifecycleJSONL appends one JSON object line to .zqk/metrics/process_lifecycle.jsonl.
func AppendProcessLifecycleJSONL(projectRoot string, row map[string]any) {
	if projectRoot == "" || row == nil {
		return
	}
	if _, ok := row["ts_rfc3339nano"]; !ok {
		row["ts_rfc3339nano"] = time.Now().UTC().Format(time.RFC3339Nano)
	}

	// Use standard stream storage for lifecycle events
	id, _ := row[objects.FieldKeyID].(string)
	_, _, err := storage.AppendToStream(projectRoot, "process_lifecycle", id, row, time.Now().UTC())
	if err != nil {
		// Log error to system logger instead of silently failing
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		logging.Fluent(logger).Warn("Failed to append to process_lifecycle stream").
			WithError(err).
			Log()
		return
	}
}

// TruncateProcessLifecycleDetail bounds optional error text in metrics rows.
func TruncateProcessLifecycleDetail(s string, max int) string {
	if max <= 0 || s == "" {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}
