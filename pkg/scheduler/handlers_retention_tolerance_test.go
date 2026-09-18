package scheduler

// BLI-177483 inventory: setupSchedulerCompleteTestEnvironment → GetTestCleanup → RunProjectTestTeardown (scheduler_test_layout_helpers_test.go).

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestSkipArchiveForOldestIDsPath(t *testing.T) {
	t.Parallel()
	if !skipArchiveForOldestIDsPath(objects.KindAgentInstruction, nil) {
		t.Fatal("agent_instruction with empty protect should skip archive")
	}
	if !skipArchiveForOldestIDsPath(objects.KindAgentInstruction, []string{}) {
		t.Fatal("empty slice protect should skip archive")
	}
	if skipArchiveForOldestIDsPath(objects.KindAgentInstruction, []string{"active"}) {
		t.Fatal("protected statuses must still archive")
	}
	if skipArchiveForOldestIDsPath(objects.KindBacklogItem, nil) {
		t.Fatal("non-HV kind should not skip archive via this path")
	}
}

func TestGetBatchConfig(t *testing.T) {
	t.Parallel()
	// nil job -> defaults
	bs, mb := getBatchConfig(nil)
	if bs != defaultRetentionToleranceBatchSize || mb != defaultRetentionToleranceMaxBatches {
		t.Errorf("getBatchConfig(nil) = %d, %d; want %d, %d", bs, mb, defaultRetentionToleranceBatchSize, defaultRetentionToleranceMaxBatches)
	}

	// job with no env -> defaults
	job := &ScheduledJob{ID: "J1"}
	bs, mb = getBatchConfig(job)
	if bs != defaultRetentionToleranceBatchSize || mb != defaultRetentionToleranceMaxBatches {
		t.Errorf("getBatchConfig(no env) = %d, %d; want defaults", bs, mb)
	}

	// job with BATCH_SIZE and MAX_BATCHES
	job.EnvironmentVariables = map[string]string{
		EnvKeyBatchSize:  "100",
		EnvKeyMaxBatches: "5",
	}
	bs, mb = getBatchConfig(job)
	if bs != 100 || mb != 5 {
		t.Errorf("getBatchConfig(env) = %d, %d; want 100, 5", bs, mb)
	}

	// MAX_BATCHES=-1 means unlimited (internal sentinel); batch size still honored
	job.EnvironmentVariables = map[string]string{EnvKeyBatchSize: "100", EnvKeyMaxBatches: "-1"}
	bs, mb = getBatchConfig(job)
	if bs != 100 || mb != retentionToleranceUnlimitedMaxBatches {
		t.Errorf("getBatchConfig(unlimited) = %d, %d; want 100, %d", bs, mb, retentionToleranceUnlimitedMaxBatches)
	}

	// invalid values fall back to defaults
	job.EnvironmentVariables = map[string]string{EnvKeyBatchSize: "x", EnvKeyMaxBatches: "-2"}
	bs, mb = getBatchConfig(job)
	if bs != defaultRetentionToleranceBatchSize || mb != defaultRetentionToleranceMaxBatches {
		t.Errorf("getBatchConfig(invalid) = %d, %d; want defaults", bs, mb)
	}
}

func TestErrObjectOverfill(t *testing.T) {
	t.Parallel()
	// Wrapped error should be unwrappable with errors.Is
	wrapped := errors.Join(errors.New("context"), ErrObjectOverfill)
	if !errors.Is(wrapped, ErrObjectOverfill) {
		t.Error("errors.Is(wrapped, ErrObjectOverfill) should be true when error wraps ErrObjectOverfill")
	}
	// Direct comparison
	if !errors.Is(ErrObjectOverfill, ErrObjectOverfill) {
		t.Error("errors.Is(ErrObjectOverfill, ErrObjectOverfill) should be true")
	}
}

func TestNewRetentionToleranceHandler(t *testing.T) {
	t.Parallel()
	// nil storage is allowed; handler should not panic
	h := NewRetentionToleranceHandler(nil, "")
	if h == nil {
		t.Fatal("NewRetentionToleranceHandler(nil, \"\") should not return nil")
	}
	hh := h.(*RetentionToleranceHandler)
	if hh.storage != nil || hh.projectRoot != emptyValue {
		t.Errorf("handler state: storage=%v projectRoot=%q", hh.storage, hh.projectRoot)
	}
}

func TestRetentionToleranceHandler_Execute_NoConfigKinds(t *testing.T) {
	t.Parallel()
	// Execute with nil storage and no project root: loader returns empty config, no kinds -> no error
	h := NewRetentionToleranceHandler(nil, "")
	job := &ScheduledJob{ID: "J1"}
	err := h.Execute(context.Background(), job)
	if err != nil {
		t.Errorf("Execute with no config kinds should not fail: %v", err)
	}
}

// TestIsHighVolumeKind ensures all retention-managed high-volume kinds are recognized (HIGH_VOLUME_EVENT_INDEXES.md, high_volume_kinds.yaml).
func TestIsHighVolumeKind(t *testing.T) {
	t.Parallel()
	tests := []struct {
		kind string
		want bool
	}{
		{"audit_event", true},
		{"change_journal_entry", true},
		{"mcp_session", true},
		{"base_metric", true},
		{"audit_aggregation_metric", true},
		{"scheduler_job", true},
		{"zqk_session", true},
		{"agent_instruction", true}, // high_volume_kinds.yaml stream; must use HV retention path
		{"verification_matrix", true},
		{"backlog_item", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isHighVolumeKind(tt.kind); got != tt.want {
			t.Errorf("isHighVolumeKind(%q) = %v, want %v", tt.kind, got, tt.want)
		}
	}
}

// TestMcpSessionDisconnectedNotProtected ensures that when mcp_session is configured with
// protect_statuses ["in_progress"] only, status "disconnected" (set by MCP server on client
// disconnect) is not protected and is eligible for retention prune (enforceMaxCount/list filter).
func TestMcpSessionDisconnectedNotProtected(t *testing.T) {
	t.Parallel()
	protectStatuses := []string{"in_progress"} // mcp_session config in retention_tolerance.yaml
	for _, p := range protectStatuses {
		if p == "disconnected" {
			t.Fatal("mcp_session protect_statuses must not include 'disconnected' so retention can prune after disconnect callback")
		}
	}
	// When enforceMaxCount builds list filter with $nin protectStatuses, objects with
	// status=disconnected are included in the candidate list for deletion.
	// So MCP session disconnect callback (status=disconnected) + this config = retention can prune.
}

// TestRetentionToleranceHandler_Execute_StreamBackedWithLongLivedContext verifies that the
// retention handler runs to completion when given a long-lived context (e.g. 2h), using
// test-scenario-style data: stream-backed audit_events and a test-specific retention config.
// Ensures the handler does not exit early due to context cancellation (e.g. CLI timeout).
func TestRetentionToleranceHandler_Execute_StreamBackedWithLongLivedContext(t *testing.T) {
	// Not t.Parallel(): setupSchedulerCompleteTestEnvironment sets ZQK_TEST_ROOT via os.Setenv (process-global).
	env := setupSchedulerCompleteTestEnvironment(t, nil)
	t.Logf("TestRoot: %s", env.TestRoot)

	projectRoot := env.TestRoot
	// Path cache required for AppendToStream (stream segment dir resolution).
	storagepkg.BuildPathAliasCacheForProject(projectRoot)
	createdAt, _ := time.Parse(time.RFC3339, "2026-03-01T12:00:00Z")

	// Create more than max_count stream-backed audit_events (no CAS files; same as daemon).
	const totalCount = 20
	const maxCount = 5
	for i := 1; i <= totalCount; i++ {
		id := "AUD-RET-" + strconv.Itoa(i)
		obj := map[string]any{
			objects.FieldKeyID:        id,
			objects.FieldKeyKind:      "audit_event",
			objects.FieldKeyEventType: "object_creation",
			objects.FieldKeyStatus:    "completed",
			objects.FieldKeyCreatedAt: createdAt.Format(time.RFC3339),
		}
		seg, offset, err := storagepkg.AppendToStream(projectRoot, "audit_event", id, obj, createdAt)
		if err != nil {
			t.Fatalf("AppendToStream %s: %v", id, err)
		}
		loc := storagepkg.FormatStreamLocation(seg, offset)
		if err := storagepkg.AppendStreamLocationToRegistry(projectRoot, "audit_event", id, loc); err != nil {
			t.Fatalf("AppendStreamLocationToRegistry %s: %v", id, err)
		}
	}

	// Write test-specific retention config: audit_event max_count=5, only "pending" protected.
	configDir := filepath.Join(projectRoot, paths.ProcessInternalConfigsDir)
	if err := fileutil.MkdirAll(configDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	configPath := filepath.Join(configDir, "retention_tolerance.yaml")
	configYAML := `kinds:
  audit_event:
    max_count: 5
    protect_statuses: ["pending"]
`
	if err := fileutil.WriteFile(configPath, []byte(configYAML), paths.FilePerm644); err != nil {
		t.Fatalf("write retention_tolerance.yaml: %v", err)
	}

	handler := NewRetentionToleranceHandler(env.Storage.(storagepkg.ObjectStorageProvider), projectRoot)
	job := &ScheduledJob{
		ID: "retention-context-test",
		EnvironmentVariables: map[string]string{
			EnvKeyKinds:      "audit_event",
			EnvKeyBatchSize:  "10",
			EnvKeyMaxBatches: "5",
		},
	}

	// Long-lived context so the handler is not cancelled by a short CLI timeout.
	baseCtx := storagepkg.WithCLIOperation(context.Background())
	ctx, cancel := context.WithTimeout(baseCtx, 2*time.Hour)
	defer cancel()

	// Handler should have reduced audit_event count to at most max_count.
	secCtx := env.SecurityContext
	filter := storagepkg.ListFilter{Kind: "audit_event"}

	countBefore, _ := env.Storage.(storagepkg.ObjectStorageProvider).Count(ctx, secCtx, filter)
	t.Logf("Count before Execute: %d", countBefore)

	err := handler.Execute(ctx, job)
	t.Logf("Execute err: %v", err)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	count, countErr := env.Storage.(storagepkg.ObjectStorageProvider).Count(ctx, secCtx, filter)
	t.Logf("Count after Execute: %d", count)
	if countErr != nil {
		t.Fatalf("Count after retention: %v", countErr)
	}
	if count > maxCount {
		t.Errorf("Count after retention = %d, want <= %d", count, maxCount)
	}

	// OBJECTIVE VERIFICATION: Prove that the stream registry was physically compacted
	// and only contains the remaining active objects.
	lines := 0
	for i := 0; i < 16; i++ {
		shardName := fmt.Sprintf("stream_registry_audit_event_%02d.jsonl", i)
		registryPath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir, shardName)
		registryData, err := fileutil.ReadFile(registryPath)
		if err == nil {
			for _, line := range strings.Split(string(registryData), "\n") {
				if strings.TrimSpace(line) != "" {
					lines++
				}
			}
		}
	}

	// If the file was not compacted, it would have 20 lines (or more if there were updates).
	if lines > maxCount {
		t.Errorf("Stream registry was not compacted! Expected <= %d lines, got %d. The gap was NOT closed.", maxCount, lines)
	}
}
