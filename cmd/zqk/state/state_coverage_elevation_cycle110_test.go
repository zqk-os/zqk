package state

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func seedStateTreeEntities(t *testing.T, sp storage.ObjectStorageProvider, sCtx context.Context, sec *pkgctx.SecurityContext) {
	t.Helper()
	org := map[string]any{objects.FieldKeyID: "ORG-STATE-001", objects.FieldKeyKind: objects.KindOrganization, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyTitle: "State Org", objects.FieldKeyDescription: "State Org Description"}
	if err := sp.Create(sCtx, sec, org); err != nil {
		t.Fatalf("create org failed: %v", err)
	}
	mis := map[string]any{objects.FieldKeyID: "MIS-STATE-001", objects.FieldKeyKind: objects.KindMission, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyTitle: "State Mission", objects.FieldKeyDescription: "State Mission Description"}
	if err := sp.Create(sCtx, sec, mis); err != nil {
		t.Fatalf("create mission failed: %v", err)
	}
	vis := map[string]any{objects.FieldKeyID: "VIS-STATE-001", objects.FieldKeyKind: objects.KindVision, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyTitle: "State Vision", objects.FieldKeyDescription: "State Vision Description"}
	if err := sp.Create(sCtx, sec, vis); err != nil {
		t.Fatalf("create vision failed: %v", err)
	}
	goal := map[string]any{objects.FieldKeyID: "GOAL-STATE-001", objects.FieldKeyKind: objects.KindGoal, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyTitle: "State Goal", objects.FieldKeyDescription: "State Goal Description"}
	if err := sp.Create(sCtx, sec, goal); err != nil {
		t.Fatalf("create goal failed: %v", err)
	}
}

func seedStateTreeHierarchy(t *testing.T, testRoot string) {
	t.Helper()
	sCtx := storage.WithSkipWriteBehind(storage.WithCLIOperation(pkgctx.WithPromoteOnCreate(pkgctx.NewSystemContext())))
	factory, err := storage.NewStorageFactory(sCtx, testRoot)
	if err != nil {
		t.Fatalf("factory init failed: %v", err)
	}
	sp := factory.GetStorage()
	sec := pkgctx.NewSystemSecurityContext()

	seedStateTreeEntities(t, sp, sCtx, sec)

	req := map[string]any{objects.FieldKeyID: "REQ-STATE-001", objects.FieldKeyKind: objects.KindRequirement, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyTitle: "State Requirement", objects.FieldKeyDescription: "State Req Description", objects.FieldKeyStatus: objects.ObjectStatusActive}
	if err := sp.Create(sCtx, sec, req); err != nil {
		t.Fatalf("create req failed: %v", err)
	}
	plan := map[string]any{objects.FieldKeyID: "PRI-STATE-001", objects.FieldKeyKind: objects.KindPriorityPlan, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyTitle: "State Plan", objects.FieldKeyDescription: "State Plan Description", objects.FieldKeyStatus: objects.ObjectStatusActive, objects.FieldKeyWorkstreamRefs: []any{"WS-STATE-001"}}
	if err := sp.Create(sCtx, sec, plan); err != nil {
		t.Fatalf("create plan failed: %v", err)
	}
	bli1 := map[string]any{objects.FieldKeyID: "BLI-STATE-001", objects.FieldKeyKind: objects.KindBacklogItem, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyTitle: "State BLI 1", objects.FieldKeyDescription: "State BLI 1 Description", objects.FieldKeyStatus: objects.ObjectStatusPlanned, "priority_tier": "P0", "claimed_by": "PER-DEV", objects.FieldKeyRequirementRefs: []any{"REQ-STATE-001"}, objects.FieldKeyPriorityPlanRef: "PRI-STATE-001"}
	if err := sp.Create(sCtx, sec, bli1); err != nil {
		t.Fatalf("create bli1 failed: %v", err)
	}
	bli2 := map[string]any{objects.FieldKeyID: "BLI-STATE-002", objects.FieldKeyKind: objects.KindBacklogItem, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion, objects.FieldKeyTitle: "State BLI 2", objects.FieldKeyDescription: "State BLI 2 Description", objects.FieldKeyStatus: objects.ObjectStatusPlanned, objects.FieldKeyRequirementRefs: []any{"REQ-UNMATCHED-001"}, objects.FieldKeyPriorityPlanRef: "PRI-STATE-001"}
	if err := sp.Create(sCtx, sec, bli2); err != nil {
		t.Fatalf("create bli2 failed: %v", err)
	}

	flushCtx, cancel := storage.DurabilityFlushContext()
	defer cancel()
	_ = storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, sp, testRoot, []string{
		objects.KindOrganization, objects.KindMission, objects.KindVision, objects.KindGoal,
		objects.KindRequirement, objects.KindPriorityPlan, objects.KindBacklogItem,
	})
	_ = sp.Shutdown(sCtx)
}

func TestStateTree_WithPopulatedHierarchy(t *testing.T) {
	testRoot := setupStateIsolatedProject(t)
	seedStateTreeHierarchy(t, testRoot)

	cmdText := newTreeCmd()
	outText, errText := executeStateCmd(t, cmdText, testRoot)
	require.NoError(t, errText)
	assert.Contains(t, outText, "ORG-STATE-001")
	assert.Contains(t, outText, "MIS-STATE-001")
	assert.Contains(t, outText, "VIS-STATE-001")
	assert.Contains(t, outText, "GOAL-STATE-001")
	assert.Contains(t, outText, "PRI-STATE-001")
	assert.Contains(t, outText, "REQ-STATE-001")
	assert.Contains(t, outText, "BLI-STATE-001")
	assert.Contains(t, outText, "BLI-STATE-002")

	cmdJSON := newTreeCmd()
	outJSON, errJSON := executeStateCmd(t, cmdJSON, testRoot, "--format", "json")
	require.NoError(t, errJSON)
	assert.Contains(t, outJSON, "ORG-STATE-001")
	assert.Contains(t, outJSON, "active_plans")
	assert.Contains(t, outJSON, "BLI-STATE-001")
	assert.Contains(t, outJSON, "BLI-STATE-002")
}

func seedRawStreamFile(t *testing.T, path string) {
	t.Helper()
	lines := []string{
		`{"id":"MUT-01","event_type":"security_event","object_ref":"SEC-01","diff_summary":"gate pass","created_at":1700000001,"created_by":"auditor"}`,
		`{"id":"MUT-02","kind":"instruction_record","operation":"reindex_cas","created_at":1700000002,"actor":"agent-x"}`,
		`{"id":"MUT-03","title":"Config Updated","created_at":1700000003}`,
		`{"id":"MUT-04","kind":"backlog_item","instruction":"execute task","created_at":1700000004}`,
		`{"id":"MUT-05","created_at":1700000005}`,
		`corrupt_line_to_skip`,
	}
	var buf bytes.Buffer
	for _, l := range lines {
		buf.WriteString(l + "\n")
	}
	if err := fileutil.WriteFile(path, buf.Bytes(), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write stream file: %v", err)
	}
}

func TestReadRecentStreamMutations_Detailed(t *testing.T) {
	assert.Nil(t, ReadRecentStreamMutations("", 10))

	testRoot := setupStateIsolatedProject(t)
	assert.Nil(t, ReadRecentStreamMutations(testRoot, 10, "nonexistent_stream_kind"))

	sDir := filepath.Join(testRoot, paths.ProjectDataDir, paths.StreamsDir, "change_journal_entry")
	if err := fileutil.MkdirAll(sDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir stream failed: %v", err)
	}
	sFile := filepath.Join(sDir, "mutations.json")
	seedRawStreamFile(t, sFile)

	mutsLimit := ReadRecentStreamMutations(testRoot, 2)
	require.Len(t, mutsLimit, 2)

	mutsCached := ReadRecentStreamMutations(testRoot, 10)
	require.NotEmpty(t, mutsCached)

	var foundSecurity, foundInstruction, foundFallback bool
	for _, m := range mutsCached {
		if m.ID == "MUT-01" {
			foundSecurity = true
			assert.Equal(t, "security_event", m.ChangeType)
		}
		if m.ID == "MUT-02" {
			foundInstruction = true
			assert.Equal(t, "instruction_record", m.ChangeType)
			assert.Equal(t, "reindex_cas", m.DiffSummary)
			assert.Equal(t, "agent-x", m.CreatedBy)
		}
		if m.ID == "MUT-05" {
			foundFallback = true
			assert.Equal(t, "event", m.ChangeType)
			assert.Equal(t, "MUT-05", m.ObjectRef)
		}
	}
	assert.True(t, foundSecurity)
	assert.True(t, foundInstruction)
	assert.True(t, foundFallback)
}

func TestStreamJournalMutations_StaticModes(t *testing.T) {
	testRoot := setupStateIsolatedProject(t)
	cmd := newStreamCmd()

	cmdBuf := new(bytes.Buffer)
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), cmdBuf)
	cmd.SetContext(ctx)

	errJSON := StreamJournalMutations(cmd, testRoot, false, 10, "json", false)
	require.NoError(t, errJSON)
	assert.Contains(t, cmdBuf.String(), "mutations")

	cmdBuf.Reset()
	errDash := StreamJournalMutations(cmd, testRoot, false, 10, "text", true)
	require.NoError(t, errDash)
	assert.Contains(t, cmdBuf.String(), "SEISMOGRAPH")

	cmdBuf.Reset()
	errSummary := StreamJournalMutations(cmd, testRoot, false, 10, "text", false)
	require.NoError(t, errSummary)
	assert.Contains(t, cmdBuf.String(), "Seismograph")

	outZero, errZero := executeStateCmd(t, cmd, testRoot, "--follow=false", "--limit=0")
	require.NoError(t, errZero)
	assert.Contains(t, outZero, "Seismograph")
}

func TestStreamJournalMutations_LiveStreamCancel(t *testing.T) {
	testRoot := setupStateIsolatedProject(t)

	sDir := filepath.Join(testRoot, paths.ProjectDataDir, paths.StreamsDir, "change_journal_entry")
	if err := fileutil.MkdirAll(sDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir stream failed: %v", err)
	}
	sFile := filepath.Join(sDir, "events.json")
	seedRawStreamFile(t, sFile)

	bufA := new(bytes.Buffer)
	baseCtxA, cancelA := context.WithCancel(context.Background())
	cancelA()
	cmdA := newStreamCmd()
	cmdA.SetContext(pkgctx.WithCommandOutputWriter(baseCtxA, bufA))

	errA := StreamJournalMutations(cmdA, testRoot, true, 10, "text", false)
	require.NoError(t, errA)
	assert.Contains(t, bufA.String(), "[Stream closed]")

	bufB := new(bytes.Buffer)
	baseCtxB, cancelB := context.WithCancel(context.Background())
	cancelB()
	cmdB := newStreamCmd()
	cmdB.SetContext(pkgctx.WithCommandOutputWriter(baseCtxB, bufB))

	errB := StreamJournalMutations(cmdB, testRoot, true, 10, "text", true)
	require.NoError(t, errB)
	assert.Contains(t, bufB.String(), "[Stream closed]")
}

func TestBuildDashboardView_EdgeCasesAndSparklines(t *testing.T) {
	mutations := make([]JournalMutation, 0, 20)
	for i := 1; i <= 20; i++ {
		mutations = append(mutations, JournalMutation{
			ID:          fmt.Sprintf("MUT-%02d", i),
			ChangeType:  "update",
			ObjectRef:   fmt.Sprintf("long_object_kind_type_%d-ITEM-%02d", i, i),
			DiffSummary: "",
			CreatedBy:   fmt.Sprintf("very_long_agent_operator_identity_%d", i),
			CreatedAt:   0,
		})
	}

	out := BuildDashboardView("/test/path", mutations)
	assert.Contains(t, out, "SEISMOGRAPH")
	assert.Contains(t, out, "...")
	assert.Contains(t, out, "--:--:--")

	assert.Equal(t, "[        ]", generateSparkline(0))
	assert.Contains(t, generateSparkline(100), "█")
}

func TestJournalCmd_FormatEdgeCases(t *testing.T) {
	testRoot := setupStateIsolatedProject(t)
	sDir := filepath.Join(testRoot, paths.ProjectDataDir, paths.StreamsDir, "change_journal_entry")
	if err := fileutil.MkdirAll(sDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir stream failed: %v", err)
	}

	longDiff := "this_is_a_very_long_diff_summary_that_exceeds_forty_characters_definitely"
	longRef := "backlog_item:BLI-EXTREMELY-LONG-IDENTIFIER-THAT-EXCEEDS-35-CHARS"
	lines := fmt.Sprintf(`{"id":"MUT-LONG-1","change_type":"update","object_ref":%q,"diff_summary":%q,"created_at":0}`+"\n", longRef, longDiff)
	lines += `{"id":"MUT-NODIFF-2","change_type":"delete","object_ref":"REQ-SHORT","diff_summary":"","created_at":0}` + "\n"

	jFile := filepath.Join(sDir, "journal_edges.json")
	if err := fileutil.WriteFile(jFile, []byte(lines), paths.FilePerm644); err != nil {
		t.Fatalf("write journal file failed: %v", err)
	}

	cmd := newJournalCmd()
	out, err := executeStateCmd(t, cmd, testRoot)
	require.NoError(t, err)
	assert.Contains(t, out, "MUT-LONG-1")
	assert.Contains(t, out, "...")
	assert.Contains(t, out, "MUT-NODIFF-2")
	assert.Contains(t, out, "delete")
}

func encodeTSDBLine(meas, tagsJSON string, nano int64, fieldsJSON string) string {
	raw := fmt.Sprintf("%s|%s|%d|%s", meas, tagsJSON, nano, fieldsJSON)
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

func seedRichTSDBData(t *testing.T, testRoot string) {
	t.Helper()
	tsdbDir := filepath.Join(testRoot, paths.ProjectDataDir, paths.SchedulerDir, "tsdb")
	if err := fileutil.MkdirAll(tsdbDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir tsdb failed: %v", err)
	}

	now := time.Now().UTC()
	var buf bytes.Buffer
	buf.WriteString(encodeTSDBLine("scheduler_job_execution", `{"job_id":"worker-job","job_type":"cron"}`, now.UnixNano(), `{"duration_seconds":0.35,"success":true}`) + "\n")
	buf.WriteString(encodeTSDBLine("scheduler_job_execution", `{"job_id":"worker-job","job_type":"cron"}`, now.Add(-time.Minute).UnixNano(), `{"duration_seconds":1.25,"success":false}`) + "\n")
	buf.WriteString(encodeTSDBLine("scheduler_job_execution", `{}`, now.Add(-2*time.Minute).UnixNano(), `{"duration_seconds":0.10,"success":true}`) + "\n")
	buf.WriteString(encodeTSDBLine("custom_telemetry", `{"job_id":"worker-job"}`, now.Add(-3*time.Minute).UnixNano(), `{"duration_seconds":0.05}`) + "\n")
	buf.WriteString(encodeTSDBLine("scheduler_job_execution", `{"job_id":"worker-job"}`, now.Add(-48*time.Hour).UnixNano(), `{"duration_seconds":0.20}`) + "\n")
	buf.WriteString("corrupted_base64_line\n")
	buf.WriteString(base64.StdEncoding.EncodeToString([]byte("short|parts")) + "\n")

	if err := fileutil.WriteFile(filepath.Join(tsdbDir, "metrics-cycle110.tsdb"), buf.Bytes(), paths.FilePerm644); err != nil {
		t.Fatalf("write tsdb file failed: %v", err)
	}

	chunkDir := filepath.Join(testRoot, paths.ProjectDataDir, "metrics", "series_lat")
	if err := fileutil.MkdirAll(chunkDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir chunk failed: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(chunkDir, "chunk_01.dat"), []byte("sample_binary_chunk"), paths.FilePerm644); err != nil {
		t.Fatalf("write chunk failed: %v", err)
	}

	draftDir := filepath.Join(testRoot, paths.ProjectDataDir, paths.ObjectDraftsDir, "backlog_item")
	if err := fileutil.MkdirAll(draftDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir draft failed: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(draftDir, "draft1.json"), []byte(`{"id":"DFT-1"}`), paths.FilePerm644); err != nil {
		t.Fatalf("write draft failed: %v", err)
	}
}

func TestRunStateTSDB_FiltersAndFlags(t *testing.T) {
	testRoot := setupStateIsolatedProject(t)
	seedRichTSDBData(t, testRoot)

	cmdA := newTsdbCmd()
	outA, errA := executeStateCmd(t, cmdA, testRoot, "--since", "today", "--job-id", "worker")
	require.NoError(t, errA)
	assert.Contains(t, outA, "worker-job")

	cmdB := newTsdbCmd()
	outB, errB := executeStateCmd(t, cmdB, testRoot, "--since", "week", "--job-id", "nonexistent")
	require.NoError(t, errB)
	assert.Contains(t, outB, "[No scheduler job execution data recorded")

	cmdC := newTsdbCmd()
	outC, errC := executeStateCmd(t, cmdC, testRoot, "--since", "month", "--format", "json")
	require.NoError(t, errC)
	assert.Contains(t, outC, "measurements")
}

func seedMultiCommandMetrics(t *testing.T, testRoot string) {
	t.Helper()
	metricsDir := filepath.Join(testRoot, paths.ProjectDataDir, "metrics")
	if err := fileutil.MkdirAll(metricsDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir metrics failed: %v", err)
	}
	var b strings.Builder
	b.WriteString(`{"metrics":{`)
	for i := 1; i <= 8; i++ {
		sep := ","
		if i == 8 {
			sep = ""
		}
		b.WriteString(fmt.Sprintf(`"cmd%d":{"command":"zqk state cmd%d","normalized_cmd":"zqk state cmd%d","invocation_count":%d,"success_count":%d,"failure_count":1,"avg_duration":50000000,"last_seen":"2026-10-20T12:00:00Z"}%s`, i, i, i, i*10, i*10-1, sep))
	}
	b.WriteString(`}}`)
	if err := fileutil.WriteFile(filepath.Join(metricsDir, "command_metrics.json"), []byte(b.String()), paths.FilePerm644); err != nil {
		t.Fatalf("write command metrics failed: %v", err)
	}
}

func TestReadTSDBTelemetry_DeepInspection(t *testing.T) {
	emptyTelem := ReadTSDBTelemetry("", 0, 0)
	assert.Empty(t, emptyTelem.Measurements)

	testRoot := setupStateIsolatedProject(t)
	seedRichTSDBData(t, testRoot)
	seedMultiCommandMetrics(t, testRoot)

	telem1 := ReadTSDBTelemetry(testRoot, 24*time.Hour, 10)
	require.NotNil(t, telem1)
	assert.Equal(t, 6, len(telem1.TopCommands))
	assert.NotEmpty(t, telem1.Measurements)
	assert.NotEmpty(t, telem1.JobSummaries)

	telemCached := ReadTSDBTelemetry(testRoot, 24*time.Hour, 10)
	require.NotNil(t, telemCached)
	assert.Equal(t, telem1.HygieneStats.TotalStorageFiles, telemCached.HygieneStats.TotalStorageFiles)
}

func TestFormatTSDBTelemetryText_StatusAndTruncation(t *testing.T) {
	telem := &TSDBTelemetry{
		TotalPoints:  15,
		TotalFiles:   2,
		DiskSizeStr:  "1.2 KB",
		Measurements: []string{},
		ChunkStats:   TSDBChunkStats{TotalChunks: 3, SeriesNames: []string{"series1", "series2"}, DiskSizeStr: "2 KB"},
		HygieneStats: TSDBHygieneStats{MaxFileDescriptors: 1024, OpenFileDescriptors: 800, FDUsagePercent: 78.1, Status: "ATTENTION", StorageSizeStr: "1 MB", TotalStorageFiles: 20, StaleLocksCount: 1, OrphanedTempCount: 2, DraftObjectsCount: 3},
		KindVolumes:  []TSDBKindVolume{{Kind: "backlog_item", CurrentCount: 5}},
		StreamRates:  []TSDBStreamRate{{StreamName: "change_journal_entry", TotalFiles: 10}},
		TopCommands:  []TSDBCommandMetric{{Command: "zqk state very long command name that exceeds thirty-six columns threshold", Invocations: 12, Failures: 2, AvgLatencyMs: 35.5}},
		JobSummaries: []TSDBJobSummary{{JobID: "job_very_long_identifier_that_exceeds_threshold", Executions: 5, Successes: 4, Failures: 1, AvgDurationMs: 120.0, MinDurationMs: 50.0, MaxDurationMs: 250.0, Sparkline: "•"}},
		RecentPoints: []TSDBPointView{{Timestamp: time.Now(), Summary: "scheduler_job_execution | worker-job"}},
	}

	outAttn := FormatTSDBTelemetryText(telem, "")
	assert.Contains(t, outAttn, "ATTENTION")
	assert.Contains(t, outAttn, "All Time")
	assert.Contains(t, outAttn, "none")
	assert.Contains(t, outAttn, "...")

	telem.HygieneStats.Status = "CRITICAL"
	telem.JobSummaries[0].LastRunAt = time.Time{}
	outCrit := FormatTSDBTelemetryText(telem, "1h")
	assert.Contains(t, outCrit, "CRITICAL")
	assert.Contains(t, outCrit, "Last 1h")
	assert.Contains(t, outCrit, "--:--:--")
}

func TestStreamJournalMutations_LiveEventDebounce(t *testing.T) {
	testRoot := setupStateIsolatedProject(t)
	sDir := filepath.Join(testRoot, paths.ProjectDataDir, paths.StreamsDir, "change_journal_entry")
	if err := fileutil.MkdirAll(sDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir stream failed: %v", err)
	}

	buf := new(bytes.Buffer)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := newStreamCmd()
	cmd.SetContext(pkgctx.WithCommandOutputWriter(ctx, buf))

	doneCh := make(chan error, 1)
	goroutinelabels.NewGoroutine("test_stream_debounce", "run live stream mutations for debounce test").StartSimple(func() {
		doneCh <- StreamJournalMutations(cmd, testRoot, true, 10, "text", false)
	})

	time.Sleep(80 * time.Millisecond)
	newFile := filepath.Join(sDir, "dynamic_entry.json")
	entry := `{"id":"MUT-DYN-01","change_type":"create","object_ref":"BLI-DYN","created_at":1700000010}` + "\n"
	if err := fileutil.WriteFile(newFile, []byte(entry), paths.FilePerm644); err != nil {
		t.Fatalf("write dynamic stream file failed: %v", err)
	}

	time.Sleep(150 * time.Millisecond)
	cancel()

	select {
	case err := <-doneCh:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for StreamJournalMutations to exit")
	}
}

func TestFormatMutationLine_SummaryFallback(t *testing.T) {
	m := JournalMutation{
		ID:         "MUT-FALLBACK",
		ChangeType: "patch",
		ObjectRef:  "REQ-FALLBACK",
		CreatedAt:  time.Now().Unix(),
	}
	line := FormatMutationLine(m)
	assert.Contains(t, line, "patch")
	assert.Contains(t, line, "MUT-FALLBACK")
}

func TestFormatStateTreeText_EmptyDiff(t *testing.T) {
	payload := &StateTreePayload{
		RecentMutations: []JournalMutation{
			{ID: "MUT-TREE-DIFF", ChangeType: "merge", ObjectRef: "PRI-TREE", DiffSummary: ""},
		},
	}
	out := FormatStateTreeText(payload)
	assert.Contains(t, out, "merge")
	assert.Contains(t, out, "MUT-TREE-DIFF")
}

func TestGenerateDurationSparkline_Flat(t *testing.T) {
	spark := generateDurationSparkline([]float64{15.0, 15.0, 15.0})
	assert.Equal(t, "→ STABLE", spark)
}

func TestReadCommandMetricsStats_InvalidJSON(t *testing.T) {
	testRoot := setupStateIsolatedProject(t)
	mDir := filepath.Join(testRoot, paths.ProjectDataDir, "metrics")
	if err := fileutil.MkdirAll(mDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir metrics failed: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(mDir, "command_metrics.json"), []byte("invalid_json_payload"), paths.FilePerm644); err != nil {
		t.Fatalf("write invalid metrics failed: %v", err)
	}
	telem := &TSDBTelemetry{}
	readCommandMetricsStats(testRoot, telem)
	assert.Empty(t, telem.TopCommands)
}
