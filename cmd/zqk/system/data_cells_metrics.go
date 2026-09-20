// data_cells_metrics.go: optional stage timings for zqk system data-cells (POL-OBS-001 async pattern).
package system

import (
	"encoding/json"
	"path/filepath"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/metricsrecording"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const dataCellsStagesJSONL = "data_cells_stages.jsonl"

// emitDataCellsStageMetrics records spec load vs output-build durations asynchronously.
// streamSummaryRead is non-zero when stream_summary/test_bundle_health.json was read (JSON envelope and/or table footer).
func emitDataCellsStageMetrics(projectRoot string, specLoad, buildOutput, streamSummaryRead time.Duration, kindFilter string) {
	goroutinelabels.NewGoroutine("data_cells_stage_metrics", "data-cells stage timing (async)").
		StartSimple(func() {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			fields := []logging.Field{
				logging.String("spec_load", specLoad.String()),
				logging.String("build_output", buildOutput.String()),
				dataCellsDurationNSField("spec_load_ns", specLoad),
				dataCellsDurationNSField("build_output_ns", buildOutput),
				logging.String("kind_filter", kindFilter),
			}
			if streamSummaryRead > 0 {
				fields = append(fields,
					logging.String("stream_summary_read", streamSummaryRead.String()),
					dataCellsDurationNSField("stream_summary_read_ns", streamSummaryRead),
				)
			}
			logging.Fluent(logger).Debug("data-cells stage timing").
				WithFields(fields...).
				Log()

			if metricsrecording.Enabled() && projectRoot != "" {
				_ = appendDataCellsStagesJSONL(projectRoot, specLoad, buildOutput, streamSummaryRead, kindFilter)
			}
		})
}

func appendDataCellsStagesJSONL(projectRoot string, specLoad, buildOutput, streamSummaryRead time.Duration, kindFilter string) error {
	dir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.MetricsDir)
	if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
		return err
	}
	p := filepath.Join(dir, dataCellsStagesJSONL)
	line, err := json.Marshal(map[string]any{
		"ts_rfc3339":            time.Now().UTC().Format(time.RFC3339Nano),
		"spec_load_ns":          specLoad.Nanoseconds(),
		"build_output_ns":       buildOutput.Nanoseconds(),
		"stream_summary_ns":     streamSummaryRead.Nanoseconds(),
		"kind_filter":           kindFilter,
		objects.FieldKeyCommand: "system data-cells",
	})
	if err != nil {
		return err
	}
	f, err := fileutil.OpenFile(p, fileutil.O_APPEND|fileutil.O_CREATE|fileutil.O_WRONLY, paths.FilePerm644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	return nil
}

func dataCellsDurationNSField(key string, d time.Duration) logging.Field {
	return logging.Field{Key: key, Value: d.Nanoseconds()}
}
