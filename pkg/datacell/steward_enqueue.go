package datacell

import (
	"path/filepath"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Steward enqueue JSONL contract (v1): one JSON object per line; drained by scheduler
// job_type data_cell_envelope_tick (see DATA_CELL_MODEL.md).

// StewardEnqueueSchemaVersionV1 is schema_version for records written by [AppendStewardEnqueueRecord].
const StewardEnqueueSchemaVersionV1 = "steward_enqueue_v1"

// StewardEnqueueMaxJSONLLineBytes caps one line read by the drain scanner (bounded memory).
const StewardEnqueueMaxJSONLLineBytes = 1024 * 1024

// Log events (POL-CODE-007 stable keys for dashboards); pair with enqueue/drain implementations.
const (
	LogEventStewardEnqueue             = "datacell_steward_enqueue"
	LogEventStewardEnqueueDrainParse   = "datacell_steward_enqueue_drain_parse"
	LogEventStewardEnqueueDrained      = "datacell_steward_enqueue_drained"
	LogEventDataCellMaintenanceEnqueue = "datacell_maintenance_enqueue"
)

// StewardEnqueueRecord is one JSON line in the steward enqueue queue (.zqk/logs/datacell/steward_enqueue.jsonl).
type StewardEnqueueRecord struct {
	SchemaVersion     string `json:"schema_version"`
	StorageProfile    string `json:"storage_profile"`
	Op                string `json:"op"`
	Detail            string `json:"detail,omitempty"`
	EnqueuedAtRFC3339 string `json:"enqueued_at_rfc3339"`
}

// BuildStewardEnqueueRecord builds a v1 record for the given profile and maintenance op.
func BuildStewardEnqueueRecord(profile StorageProfile, op MaintenanceOp) StewardEnqueueRecord {
	return StewardEnqueueRecord{
		SchemaVersion:     StewardEnqueueSchemaVersionV1,
		StorageProfile:    string(profile),
		Op:                op.Name,
		Detail:            op.Detail,
		EnqueuedAtRFC3339: time.Now().UTC().Format(time.RFC3339),
	}
}

// AppendStewardEnqueueRecord appends one JSON line to the steward enqueue JSONL file for projectRoot.
func AppendStewardEnqueueRecord(projectRoot string, rec StewardEnqueueRecord) error {
	root := filepath.Clean(projectRoot)
	if root == "" || root == "." {
		return errfmt.Errorf("append steward enqueue: project root required")
	}
	path := StewardEnqueueJSONLPath(root)
	if path == "" {
		return errfmt.Errorf("append steward enqueue: empty jsonl path")
	}
	if err := fileutil.AppendJSONLine(path, rec); err != nil {
		return errfmt.Errorf("append steward enqueue: %w", err)
	}
	recordStewardMetricsEnqueue(root, rec)
	return nil
}
