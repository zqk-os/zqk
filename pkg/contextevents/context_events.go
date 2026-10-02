// Package contextevents provides append-only JSONL for lightweight inter-subsystem context signals
// (criteria hints, matrix outcomes, manual notes) under .zqk/metrics/, following POL-OBS-001 like
// data_cell_envelope_tick metrics and steward metrics.
//
// This is the durable evidence path — not in-process fan-out (use pkg/coordination for that).
// See docs/architecture/CEF_EVENT_PATH_AND_GLOBALS.md.
package contextevents

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Sentinel errors use object field wire names where the error concerns a single key.
var (
	ErrPayloadTooLarge = errors.New(objects.FieldKeyPayload + ": JSON exceeds max size")
	ErrEmptyEventType  = errors.New(objects.FieldKeyEventType + ": required")
)

const (
	// SchemaVersionV1 is the wire format version for context_events.jsonl lines.
	SchemaVersionV1 = "1"
	// DefaultSourceCLI is the default Source when operators use the emit CLI.
	DefaultSourceCLI = "cli"
)

const (
	noteMaxRunes        = 4096
	payloadJSONMaxBytes = 65536
)

var appendMu sync.Mutex

// ContextEventsJSONLPath returns .zqk/metrics/context_events.jsonl for projectRoot.
func ContextEventsJSONLPath(projectRoot string) string {
	if projectRoot == "" {
		return ""
	}
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.MetricsDir, paths.ContextEventsJSONLFile)
}

// Record is one JSON line in context_events.jsonl. Encoding uses objects.FieldKey* + wire constants
// (see json_codec.go); do not duplicate field strings in callers.
type Record struct {
	TsRFC3339             string
	SchemaVersion         string
	EventType             string
	Source                string
	CorrelationID         string
	JobID                 string
	Note                  string
	CriteriaRefs          []string
	BacklogItemRefs       []string
	ConvergenceSessionRef string
	Payload               map[string]any
}

// Append marshals rec and appends one line to context_events.jsonl. Errors are returned for tests;
// production callers often ignore errors (best-effort observability).
func Append(projectRoot string, rec *Record) error {
	if projectRoot == "" || rec == nil {
		return nil
	}
	rec.EventType = strings.TrimSpace(rec.EventType)
	if rec.EventType == "" {
		return ErrEmptyEventType
	}
	if rec.SchemaVersion == "" {
		rec.SchemaVersion = SchemaVersionV1
	}
	if rec.TsRFC3339 == "" {
		rec.TsRFC3339 = time.Now().UTC().Format(time.RFC3339Nano)
	}
	rec.Note = truncateNote(rec.Note)
	normalizeSliceFields(rec)

	dir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.MetricsDir)
	p := filepath.Join(dir, paths.ContextEventsJSONLFile)
	appendMu.Lock()
	defer appendMu.Unlock()
	return fileutil.AppendJSONLine(p, rec)
}

func truncateNote(s string) string {
	if s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) <= noteMaxRunes {
		return s
	}
	return string(r[:noteMaxRunes])
}

func normalizeSliceFields(rec *Record) {
	rec.CriteriaRefs = trimSlice(rec.CriteriaRefs)
	rec.BacklogItemRefs = trimSlice(rec.BacklogItemRefs)
}

func trimSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// MergePayloadJSON merges raw JSON object bytes into dst (typically rec.Payload). Duplicate keys
// from json overwrite prior map entries. Oversized input returns an error without mutating dst.
func MergePayloadJSON(dst map[string]any, raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return dst, nil
	}
	if len(raw) > payloadJSONMaxBytes {
		return dst, ErrPayloadTooLarge
	}
	raw = trimSpaceBytes(raw)
	if len(raw) == 0 {
		return dst, nil
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return dst, err
	}
	if dst == nil {
		dst = make(map[string]any, len(obj))
	}
	for k, v := range obj {
		dst[k] = v
	}
	return dst, nil
}

func trimSpaceBytes(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}
