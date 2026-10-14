package state

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func setupStateIsolatedProject(t *testing.T) string {
	t.Helper()
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind: "cmd.state.unit",
	})
	return proj.Root
}

func executeStateCmd(t *testing.T, cmd *cobra.Command, projectRoot string, args ...string) (string, error) {
	return executeStateCmdWithContext(t, pkgctx.NewSystemContext(), cmd, projectRoot, args...)
}

func executeStateCmdWithContext(t *testing.T, baseCtx context.Context, cmd *cobra.Command, projectRoot string, args ...string) (string, error) {
	t.Helper()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if projectRoot != "" {
		cli.SetContext(cmd, cli.ContextForProjectRoot(projectRoot))
	}
	cmd.SetContext(pkgctx.WithCommandOutputWriter(baseCtx, buf))
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestNewStateCmd_Structure(t *testing.T) {
	t.Parallel()

	cmd := NewStateCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "state", cmd.Name())

	subcommands := cmd.Commands()
	subNames := make(map[string]bool)
	for _, sub := range subcommands {
		subNames[sub.Name()] = true
	}

	assert.True(t, subNames["tree"], "state command must include 'tree'")
	assert.True(t, subNames["journal"], "state command must include 'journal'")
	assert.True(t, subNames["stream"], "state command must include 'stream'")
	assert.True(t, subNames["tsdb"], "state command must include 'tsdb'")
}

func TestStateTreeCmd_TextAndJSON(t *testing.T) {
	testRoot := setupStateIsolatedProject(t)

	// 1. Text rendering
	cmdText := newTreeCmd()
	outText, errText := executeStateCmd(t, cmdText, testRoot)
	require.NoError(t, errText)
	assert.Contains(t, outText, "Kernel State Tree")
	assert.Contains(t, outText, "Organization:")
	assert.Contains(t, outText, "Priority Plan:")

	// 2. JSON rendering
	cmdJSON := newTreeCmd()
	outJSON, errJSON := executeStateCmd(t, cmdJSON, testRoot, "--format", "json")
	require.NoError(t, errJSON)
	assert.NotEmpty(t, outJSON)
}

func TestStateJournalCmd_TextAndJSON(t *testing.T) {
	testRoot := setupStateIsolatedProject(t)

	// Seed fake journal mutations into tracked streams directory
	streamDir := filepath.Join(testRoot, paths.ProjectDataDir, paths.StreamsDir, "change_journal_entry")
	require.NoError(t, fileutil.MkdirAll(streamDir, paths.DirPerm755))
	journalFile := filepath.Join(streamDir, "test_journal.json")

	entry1 := `{"id":"MUT-1","change_type":"create","object_ref":"BLI-001","diff_summary":"Created item","created_at":1700000000}` + "\n"
	entry2 := `{"id":"MUT-2","change_type":"update","object_ref":"REQ-001","diff_summary":"Updated status to validated","created_at":1700000100}` + "\n"
	require.NoError(t, fileutil.WriteFile(journalFile, []byte(entry1+entry2), paths.FilePerm644))

	// 1. Default text format
	cmd1 := newJournalCmd()
	out1, err1 := executeStateCmd(t, cmd1, testRoot, "--limit", "10")
	require.NoError(t, err1)
	assert.Contains(t, out1, "Recent Change Journal Mutations")
	assert.Contains(t, out1, "MUT-1")
	assert.Contains(t, out1, "BLI-001")
	assert.Contains(t, out1, "MUT-2")

	// 2. JSON format
	cmd2 := newJournalCmd()
	out2, err2 := executeStateCmd(t, cmd2, testRoot, "--format", "json")
	require.NoError(t, err2)
	assert.Contains(t, out2, "\"mutations\"")
	assert.Contains(t, out2, "MUT-1")

	// 3. Empty journal
	emptyRoot := setupStateIsolatedProject(t)
	cmd3 := newJournalCmd()
	out3, err3 := executeStateCmd(t, cmd3, emptyRoot)
	require.NoError(t, err3)
	assert.Contains(t, out3, "Recent Change Journal Mutations (Total: 0)")
}

func TestFormatEventBadge(t *testing.T) {
	t.Parallel()

	assert.Contains(t, FormatEventBadge("create"), "CREATE")
	assert.Contains(t, FormatEventBadge("update"), "UPDATE")
	assert.Contains(t, FormatEventBadge("delete"), "DELETE")
	assert.Contains(t, FormatEventBadge("rollback"), "ROLLBACK")
	assert.Contains(t, FormatEventBadge("custom_event"), "CUSTOM_E")
}

func TestFormatMutationLine(t *testing.T) {
	t.Parallel()

	m1 := JournalMutation{
		ID:          "MUT-100",
		ChangeType:  "create",
		ObjectRef:   "backlog_item:BLI-001",
		DiffSummary: "Created backlog item",
		CreatedAt:   time.Now().Unix(),
	}
	line1 := FormatMutationLine(m1)
	assert.Contains(t, line1, "MUT-100")
	assert.Contains(t, line1, "CREATE")

	m2 := JournalMutation{
		ID:          "MUT-200",
		ChangeType:  "custom",
		ObjectRef:   "long_object_reference_that_exceeds_threshold_and_gets_truncated",
		DiffSummary: "very long diff summary that will also be truncated to preserve alignment",
		CreatedAt:   0,
	}
	line2 := FormatMutationLine(m2)
	assert.Contains(t, line2, "MUT-200")
	assert.Contains(t, line2, "...")
}

func TestFormatByteSizeAndSparkline(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "500 B", formatByteSize(500))
	assert.Contains(t, formatByteSize(2048), "KB")
	assert.Contains(t, formatByteSize(10*1024*1024), "MB")
	assert.Contains(t, formatByteSize(5*1024*1024*1024), "GB")

	spark := generateDurationSparkline([]float64{10.0, 50.0, 100.0, 20.0})
	assert.NotEmpty(t, spark)

	assert.Equal(t, "--", generateDurationSparkline(nil))
	assert.Equal(t, "• BASE", generateDurationSparkline([]float64{10.0}))
}

func TestReadTSDBTelemetry_Sections(t *testing.T) {
	testRoot := setupStateIsolatedProject(t)

	// 1. Seed command metrics JSON payload
	metricsDir := filepath.Join(testRoot, paths.ProjectDataDir, "metrics")
	require.NoError(t, fileutil.MkdirAll(metricsDir, paths.DirPerm755))
	cmdMetricsJSON := `{
		"metrics": {
			"zqk state tree": {
				"command": "zqk state tree",
				"normalized_cmd": "zqk state tree",
				"invocation_count": 10,
				"success_count": 9,
				"failure_count": 1,
				"avg_duration": 45000000,
				"last_seen": "2026-10-14T20:00:00Z"
			},
			"zqk state journal": {
				"command": "zqk state journal",
				"normalized_cmd": "zqk state journal",
				"invocation_count": 5,
				"success_count": 5,
				"failure_count": 0,
				"avg_duration": 12000000,
				"last_seen": "2026-10-14T20:01:00Z"
			}
		}
	}`
	require.NoError(t, fileutil.WriteFile(filepath.Join(metricsDir, "command_metrics.json"), []byte(cmdMetricsJSON), paths.FilePerm644))

	// 2. Seed process objects
	processDir := filepath.Join(testRoot, paths.ProjectDataDir, paths.ProcessDir, "backlog_item")
	require.NoError(t, fileutil.MkdirAll(processDir, paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(processDir, "sample.json"), []byte(`{"id":"BLI-001"}`), paths.FilePerm644))

	// 3. Seed streams
	streamDir := filepath.Join(testRoot, paths.ProjectDataDir, paths.StreamsDir, "change_journal_entry")
	require.NoError(t, fileutil.MkdirAll(streamDir, paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(streamDir, "stream-1.json"), []byte(`{"id":"CHA-001"}`), paths.FilePerm644))

	// 4. Seed tsdb data
	tsdbDir := filepath.Join(testRoot, paths.ProjectDataDir, paths.SchedulerDir, "tsdb")
	require.NoError(t, fileutil.MkdirAll(tsdbDir, paths.DirPerm755))
	tsdbLine := `{"time":"2026-10-14T20:00:00Z","measurement":"latency","val":42.5}` + "\n"
	require.NoError(t, fileutil.WriteFile(filepath.Join(tsdbDir, "metrics-001.tsdb"), []byte(tsdbLine), paths.FilePerm644))

	// Run ReadTSDBTelemetry
	telemetry := ReadTSDBTelemetry(testRoot, 1*time.Hour, 10)
	require.NotNil(t, telemetry)
	assert.NotEmpty(t, telemetry.TopCommands)
	assert.NotEmpty(t, telemetry.KindVolumes)
	assert.NotEmpty(t, telemetry.StreamRates)

	// Format text
	rendered := FormatTSDBTelemetryText(telemetry, "1h")
	assert.Contains(t, rendered, "Time-Series Database")
	assert.Contains(t, rendered, "zqk state tree")
}

func TestFormatEventBadge_AllKinds(t *testing.T) {
	t.Parallel()

	testCases := map[string]string{
		"SYSTEM_CONFIG_CHANGE":    "⚡ CONFIG",
		"COMMAND_EXECUTION":       "⚡ EXEC",
		"SCHEDULER_JOB_COMPLETED": "⚡ JOB_DONE",
		"OBJECT_CREATION":         "⚡ CREATE",
		"OBJECT_UPDATE":           "⚡ UPDATE",
		"OBJECT_DELETION":         "⚡ DELETE",
		"PROCESS_LIFECYCLE":       "⚡ PROC",
		"AGENT_INSTRUCTION":       "⚡ INSTR",
		"custom_event_long":       "⚡ CUSTOM_E",
		"custom":                  "⚡ CUSTOM",
	}

	for input, expected := range testCases {
		assert.Equal(t, expected, FormatEventBadge(input))
	}
}

func TestStateTsdbCmd_ExecutionOptions(t *testing.T) {
	testRoot := setupStateIsolatedProject(t)

	// Test default text execution
	cmd1 := newTsdbCmd()
	out1, err1 := executeStateCmd(t, cmd1, testRoot)
	require.NoError(t, err1)
	assert.Contains(t, out1, "Time-Series Database")

	// Test json format and since flags
	cmd2 := newTsdbCmd()
	out2, err2 := executeStateCmd(t, cmd2, testRoot, "--format", "json", "--since", "24h", "--job-id", "test-job", "--limit", "5")
	require.NoError(t, err2)
	assert.Contains(t, out2, "measurements")
}

func TestStateStreamCmd_ExecutionNoFollow(t *testing.T) {
	testRoot := setupStateIsolatedProject(t)

	cmd := newStreamCmd()
	out, err := executeStateCmd(t, cmd, testRoot, "--follow=false", "--dashboard=false")
	require.NoError(t, err)
	assert.Contains(t, out, "Kernel State Seismograph")
}

func TestFormatDurationMs_Variations(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "0.45ms", formatDurationMs(0.452))
	assert.Equal(t, "45.2ms", formatDurationMs(45.2))
	assert.Equal(t, "2.50s", formatDurationMs(2500.0))
	assert.Equal(t, "90.00s", formatDurationMs(90000.0))
}
