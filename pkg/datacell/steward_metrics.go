package datacell

import (
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/lanceman/zqk/pkg/metricsrecording"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

const stewardMetricsJSONLFile = "data_cell_steward.jsonl"

// StewardMetricsJSONLPath returns the append-only metrics JSONL file for data-cell steward
// enqueue/drain events under .zqk/metrics/ (same subtree as envelope-tick metrics in pkg/scheduler).
// Lines are written only when metrics recording is enabled; the file may be absent until first write.
func StewardMetricsJSONLPath(projectRoot string) string {
	if projectRoot == "" {
		return ""
	}
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.MetricsDir, stewardMetricsJSONLFile)
}

// stewardMetricDetailMax bounds optional detail text in metrics JSONL rows (bytes).
const stewardMetricDetailMax = 256

func truncateStewardMetricDetail(s string) string {
	if len(s) <= stewardMetricDetailMax {
		return s
	}
	return s[:stewardMetricDetailMax]
}

// JSONL row shapes (struct tags hold wire names; avoid map[string]any + field-key literal sites
// and an import cycle: pkg/objects already depends on pkg/datacell).
type stewardEnqueueMetricRow struct {
	TsRFC3339      string `json:"ts_rfc3339"`
	Event          string `json:"event"`
	SchemaVersion  string `json:"schema_version"`
	StorageProfile string `json:"storage_profile"`
	Op             string `json:"op"`
	Detail         string `json:"detail,omitempty"`
}

type stewardDrainMetricRow struct {
	TsRFC3339                 string `json:"ts_rfc3339"`
	Event                     string `json:"event"`
	LinesDrained              int    `json:"lines_drained"`
	ParseErrors               int    `json:"parse_errors"`
	DataCellEnvelopeTickJobID string `json:"data_cell_envelope_tick_job_id,omitempty"`
}

// appendStewardMetricsJSONL appends one JSON line under .zqk/metrics/ when metrics recording is enabled.
// Mirrors pkg/scheduler appendDataCellEnvelopeTickJSONL (POL-OBS-001); best-effort — errors ignored by callers.
func appendStewardMetricsJSONL(projectRoot string, row any) error {
	if projectRoot == "" || !metricsrecording.Enabled() {
		return nil
	}
	dir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.MetricsDir)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		return err
	}
	p := filepath.Join(dir, stewardMetricsJSONLFile)
	line, err := json.Marshal(row)
	if err != nil {
		return err
	}
	f, err := fileutil.OpenFile(p, fileutil.O_APPEND|fileutil.O_CREATE|fileutil.O_WRONLY, paths.FilePerm644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = f.Write(append(line, '\n'))
	return err
}

func recordStewardMetricsEnqueue(projectRoot string, rec StewardEnqueueRecord) {
	row := stewardEnqueueMetricRow{
		TsRFC3339:      time.Now().UTC().Format(time.RFC3339Nano),
		Event:          "steward_enqueue",
		SchemaVersion:  rec.SchemaVersion,
		StorageProfile: rec.StorageProfile,
		Op:             rec.Op,
	}
	if rec.Detail != "" {
		row.Detail = truncateStewardMetricDetail(rec.Detail)
	}
	_ = appendStewardMetricsJSONL(projectRoot, row)
}

func recordStewardMetricsDrain(projectRoot, envelopeTickJobID string, drainedRecords, parseErrors int) {
	if projectRoot == "" || !metricsrecording.Enabled() {
		return
	}
	if drainedRecords == 0 && parseErrors == 0 {
		return
	}
	row := stewardDrainMetricRow{
		TsRFC3339:    time.Now().UTC().Format(time.RFC3339Nano),
		Event:        "steward_drain",
		LinesDrained: drainedRecords,
		ParseErrors:  parseErrors,
	}
	if envelopeTickJobID != "" {
		row.DataCellEnvelopeTickJobID = envelopeTickJobID
	}
	_ = appendStewardMetricsJSONL(projectRoot, row)
}
