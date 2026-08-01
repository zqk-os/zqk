package scheduler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
)

// CVS measurement JSONL: event_type values for consumers (e.g. cursor-hook-notify watchRoutes filter).
const (
	CVSEventTypeMeasureApplied = "cvs_measure_applied"
	// CVSEventTypeMeasureNoNewWatermark is emitted when measure ran but only an activity_log audit was written
	// (health tail watermark unchanged vs CVS last_measurement_at).
	CVSEventTypeMeasureNoNewWatermark = "cvs_measure_no_new_watermark"
	// KeyCVSEventType is the JSON field for filtering (aligns with test-bundles events.jsonl event_type).
	KeyCVSEventType = "event_type"
)

const cvsMeasurementHintMaxRunes = 400

var cvsMeasurementEventsMu sync.Mutex

// AppendCVSMeasurementEvent appends one JSON line to CVSMeasurementEventsFilePath. Best-effort; errors ignored.
// Serialized with cvsMeasurementEventsMu so concurrent ticks do not interleave lines.
func AppendCVSMeasurementEvent(projectRoot string, entry map[string]any) {
	if projectRoot == emptyValue || entry == nil {
		return
	}
	if entry[KeyCVSEventType] == nil {
		entry[KeyCVSEventType] = CVSEventTypeMeasureApplied
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	path := CVSMeasurementEventsFilePath(projectRoot)
	cvsMeasurementEventsMu.Lock()
	defer cvsMeasurementEventsMu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, paths.FilePerm644)
	if err != nil {
		return
	}
	if _, err := f.Write(data); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		SLog(logger).Error("Failed to write CVS measurement event data", err).Log()
	}
	if _, err := f.WriteString("\n"); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		SLog(logger).Error("Failed to write CVS measurement event newline", err).Log()
	}
	if err := f.Close(); err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		SLog(logger).Error("Failed to close CVS measurement event log file", err).Log()
	}
}

func truncateCVSHint(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	if maxRunes <= 0 || s == emptyValue {
		return s
	}
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "…"
}

// cvsMeasurementEventsEnabled returns false when job env sets CVS_MEASUREMENT_EVENTS to 0 or false (case-insensitive).
func cvsMeasurementEventsEnabled(job *ScheduledJob) bool {
	if scheduledJobEnvMissing(job) {
		return true
	}
	v := strings.TrimSpace(strings.ToLower(envLookup(job, EnvKeyCVSMeasurementEvents)))
	if v == "0" || v == "false" || v == "no" {
		return false
	}
	return true
}
