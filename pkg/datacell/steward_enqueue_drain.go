package datacell

import (
	"bufio"
	"encoding/json"
	"strings"

	"github.com/lanceman/zqk/pkg/logging"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// DrainStewardEnqueueLog reads and removes the steward enqueue JSONL file, logging each valid record.
// envelopeTickJobID is optional; when non-empty it is written to steward metrics on drain (link to SCH-dce-tick).
// Returns the number of successfully parsed records (lines with valid JSON). Missing file is not an error.
func DrainStewardEnqueueLog(projectRoot string, logger logging.Logger, envelopeTickJobID string) (int, error) {
	path := StewardEnqueueJSONLPath(projectRoot)
	if path == "" {
		return 0, nil
	}
	f, err := fileutil.Open(path)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	sc := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, StewardEnqueueMaxJSONLLineBytes)

	drainedRecords := 0
	parseErrors := 0
	physicalLine := 0
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		physicalLine++
		var rec StewardEnqueueRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			parseErrors++
			if logger != nil {
				logging.Fluent(logger).Warn(LogEventStewardEnqueueDrainParse).
					Path(path).
					Int("line", physicalLine).
					WithError(err).
					Log()
			}
			continue
		}
		drainedRecords++
		if logger != nil {
			logging.Fluent(logger).Info(LogEventStewardEnqueueDrained).
				String("schema_version", rec.SchemaVersion).
				String("storage_profile", rec.StorageProfile).
				String("op", rec.Op).
				String("detail", rec.Detail).
				String("enqueued_at", rec.EnqueuedAtRFC3339).
				Log()
		}
	}
	if err := sc.Err(); err != nil {
		_ = f.Close()
		return drainedRecords, err
	}
	if err := f.Close(); err != nil {
		return drainedRecords, err
	}
	_ = fileutil.Remove(path)
	recordStewardMetricsDrain(projectRoot, envelopeTickJobID, drainedRecords, parseErrors)
	return drainedRecords, nil
}
