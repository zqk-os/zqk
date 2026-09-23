package state

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestReadTSDBTelemetry_AndFormat(t *testing.T) {
	tempRoot := t.TempDir()

	// Setup TSDB dir
	tsdbDir := filepath.Join(tempRoot, paths.ProjectDataDir, paths.SchedulerDir, "tsdb")
	require.NoError(t, fileutil.MkdirAll(tsdbDir, paths.DirPerm755))

	// Setup Metrics dir with a chunk file
	chunkDir := filepath.Join(tempRoot, paths.ProjectDataDir, "metrics", "command_metrics")
	require.NoError(t, fileutil.MkdirAll(chunkDir, paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(chunkDir, "cmd_test.chunk"), []byte("sample_chunk_data"), paths.FilePerm644))

	// Write simulated TSDB points
	now := time.Now().UTC()
	pt1 := fmt.Sprintf("scheduler_job_execution|%s|%d|%s",
		mustJSON(map[string]string{"job_id": "SCH-test-job", "job_type": "maintenance", "status": "success"}),
		now.UnixNano(),
		mustJSON(map[string]any{"duration_seconds": 1.25, "success": true}),
	)
	pt2 := fmt.Sprintf("scheduler_job_execution|%s|%d|%s",
		mustJSON(map[string]string{"job_id": "SCH-test-job", "job_type": "maintenance", "status": "failed"}),
		now.Add(-time.Minute).UnixNano(),
		mustJSON(map[string]any{"duration_seconds": 2.50, "success": false}),
	)

	b64Line1 := base64.StdEncoding.EncodeToString([]byte(pt1))
	b64Line2 := base64.StdEncoding.EncodeToString([]byte(pt2))
	content := b64Line1 + "\n" + b64Line2 + "\n"

	tsdbFile := filepath.Join(tsdbDir, "metrics-2026-09-22.tsdb")
	require.NoError(t, fileutil.WriteFile(tsdbFile, []byte(content), paths.FilePerm644))

	// Read telemetry
	telem := ReadTSDBTelemetry(tempRoot, 24*time.Hour, 10)
	require.NotNil(t, telem)
	assert.Equal(t, 2, telem.TotalPoints)
	assert.Equal(t, 1, telem.TotalFiles)
	assert.Contains(t, telem.Measurements, "scheduler_job_execution")
	assert.Equal(t, 1, telem.ChunkStats.TotalChunks)
	assert.Contains(t, telem.ChunkStats.SeriesNames, "command_metrics")

	require.Len(t, telem.JobSummaries, 1)
	js := telem.JobSummaries[0]
	assert.Equal(t, "SCH-test-job", js.JobID)
	assert.Equal(t, 2, js.Executions)
	assert.Equal(t, 1, js.Successes)
	assert.Equal(t, 1, js.Failures)
	assert.Equal(t, 50.0, js.SuccessRate)
	assert.InDelta(t, 1875.0, js.AvgDurationMs, 0.1)
	assert.NotEmpty(t, js.Sparkline)

	// Test text formatting
	text := FormatTSDBTelemetryText(telem, "24h")
	assert.Contains(t, text, "Time-Series Database (TSDB) Telemetry")
	assert.Contains(t, text, "SCH-test-job")
	assert.Contains(t, text, "Last 24h")
	assert.Contains(t, text, "command_metrics")
}

func TestStateTsdbCmd_Execution(t *testing.T) {
	tempRoot := t.TempDir()
	tsdbDir := filepath.Join(tempRoot, paths.ProjectDataDir, paths.SchedulerDir, "tsdb")
	require.NoError(t, fileutil.MkdirAll(tsdbDir, paths.DirPerm755))

	now := time.Now().UTC()
	pt := fmt.Sprintf("scheduler_job_execution|%s|%d|%s",
		mustJSON(map[string]string{"job_id": "SCH-cap-orch", "job_type": "cap", "status": "success"}),
		now.UnixNano(),
		mustJSON(map[string]any{"duration_seconds": 0.5, "success": true}),
	)
	b64 := base64.StdEncoding.EncodeToString([]byte(pt))
	require.NoError(t, fileutil.WriteFile(filepath.Join(tsdbDir, "metrics-2026-09-22.tsdb"), []byte(b64+"\n"), paths.FilePerm644))

	// Test Text output
	testCmd := &cobra.Command{}
	var outBuf bytes.Buffer
	testCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &outBuf))

	err := RunStateTSDB(testCmd, tempRoot, "24h", "", 10, "text")
	require.NoError(t, err)

	out := outBuf.String()
	assert.Contains(t, out, "SCH-cap-orch")
	assert.Contains(t, out, "scheduler_job_execution")

	// Test JSON output
	jsonCmd := &cobra.Command{}
	var jsonBuf bytes.Buffer
	jsonCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &jsonBuf))

	err = RunStateTSDB(jsonCmd, tempRoot, "24h", "SCH-cap", 10, "json")
	require.NoError(t, err)

	var payload TSDBTelemetry
	err = json.Unmarshal(jsonBuf.Bytes(), &payload)
	require.NoError(t, err)
	assert.Equal(t, 1, payload.TotalPoints)
	assert.Len(t, payload.JobSummaries, 1)
	assert.Equal(t, "SCH-cap-orch", payload.JobSummaries[0].JobID)
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
