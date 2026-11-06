package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/accumulator"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestWhatsNextCommand_SyncSweepFlagRegistered(t *testing.T) {
	cmd := NewWhatsNextCmd()
	flag := cmd.Flags().Lookup("sync-sweep")
	if flag == nil {
		t.Fatal("expected --sync-sweep flag to be registered on workflow whats-next command")
	}
	if flag.DefValue != "false" {
		t.Fatalf("expected --sync-sweep default value to be 'false', got %q", flag.DefValue)
	}
}

func TestRunWhatsNextLite_ZeroCostHotPathLatency(t *testing.T) {
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.StateDir)
	if err := fileutil.MkdirAll(stateDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	litePath := filepath.Join(stateDir, "whats_next_lite.json")
	payload := &whatsnext.WhatsNextLitePayload{
		SchemaVersion:  whatsnext.MaterializedViewSchemaVersion,
		MaterializedAt: time.Now().UTC(),
		LeadPlan: &whatsnext.WhatsNextPriorityPlan{
			ID:     "PRI-TEST-LEAD",
			Title:  "Lead Plan Title",
			Status: objects.ObjectStatusInProgress,
		},
		ActivePlans: []whatsnext.WhatsNextPriorityPlan{
			{
				ID:     "PRI-TEST-LEAD",
				Title:  "Lead Plan Title",
				Status: objects.ObjectStatusInProgress,
			},
		},
		BacklogCountsByPlan: map[string]map[string]int{
			"PRI-TEST-LEAD": {
				objects.ObjectStatusPlanned:    5,
				objects.ObjectStatusInProgress: 2,
			},
		},
		TotalBacklogCounts: map[string]int{
			objects.ObjectStatusPlanned:    5,
			objects.ObjectStatusInProgress: 2,
		},
		RunwayDepth: 3,
		ActiveTasksByAssignee: map[string]*whatsnext.ActiveTaskNode{
			"PER-AGENT-1": {
				ID:     "ATK-TASK-1",
				Status: objects.ObjectStatusInProgress,
				Title:  "Test Active Task",
			},
		},
		AgentInstructionsByPlan: map[string]string{
			"PRI-TEST-LEAD": "continue",
		},
		PackagingCuesByPlan: map[string]string{
			"PRI-TEST-LEAD": "wrap-it-up",
		},
	}

	env := accumulator.Envelope[*whatsnext.WhatsNextLitePayload]{
		SchemaVersion:  whatsnext.MaterializedViewSchemaVersion,
		MaterializedAt: time.Now().UTC(),
		Payload:        payload,
	}
	data, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		t.Fatalf("marshal envelope failed: %v", err)
	}
	if err := fileutil.WriteFile(litePath, data, paths.FilePerm644); err != nil {
		t.Fatalf("write lite file failed: %v", err)
	}

	start := time.Now()
	loaded, err := whatsnext.GetOrRecoverPayload(context.Background(), nil, tmpDir, 2*time.Minute)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected non-nil loaded payload")
	}

	// Performance verification: Hot read must complete fast (<= 100ms in unit test environment)
	if duration > 100*time.Millisecond {
		t.Errorf("hot path read exceeded latency target: took %v (expected <= 100ms in unit test)", duration)
	}

	if loaded.Stale {
		t.Errorf("expected fresh view, got Stale=true")
	}
	if loaded.LeadPlan == nil || loaded.LeadPlan.ID != "PRI-TEST-LEAD" {
		t.Errorf("expected lead plan PRI-TEST-LEAD, got %v", loaded.LeadPlan)
	}
	if loaded.RunwayDepth != 3 {
		t.Errorf("expected runway depth 3, got %d", loaded.RunwayDepth)
	}
	if counts, ok := loaded.BacklogCountsByPlan["PRI-TEST-LEAD"]; !ok || counts[objects.ObjectStatusPlanned] != 5 {
		t.Errorf("expected 5 planned backlog items, got %v", counts)
	}
}

func TestRunWhatsNextLite_ColdBootNonBlockingRecovery(t *testing.T) {
	tmpDir := t.TempDir()

	start := time.Now()
	loaded, err := whatsnext.GetOrRecoverPayload(context.Background(), nil, tmpDir, 2*time.Minute)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error on cold boot: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected bootstrap payload on cold boot")
	}

	// Cold boot read must also complete quickly without blocking caller
	if duration > 10*time.Millisecond {
		t.Errorf("cold boot check took %v (expected <= 10ms)", duration)
	}

	if !loaded.Stale {
		t.Error("expected Stale=true on cold boot")
	}
	if !loaded.Recovering {
		t.Error("expected Recovering=true on cold boot")
	}
}

func TestRunWhatsNext_ActiveInProgressPlan_HotPathFast(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	stateDir := filepath.Join(proj.Root, paths.ProjectDataDir, paths.StateDir)
	require.NoError(t, fileutil.MkdirAll(stateDir, paths.DirPerm755))

	litePath := filepath.Join(stateDir, "whats_next_lite.json")
	payload := &whatsnext.WhatsNextLitePayload{
		SchemaVersion:  whatsnext.MaterializedViewSchemaVersion,
		MaterializedAt: time.Now().UTC(),
		LeadPlan: &whatsnext.WhatsNextPriorityPlan{
			ID:     "PRI-ACTIVE",
			Title:  "Active In Progress Plan",
			Status: objects.ObjectStatusInProgress,
		},
		ActivePlans: []whatsnext.WhatsNextPriorityPlan{
			{
				ID:     "PRI-ACTIVE",
				Title:  "Active In Progress Plan",
				Status: objects.ObjectStatusInProgress,
			},
		},
		BacklogCountsByPlan: map[string]map[string]int{
			"PRI-ACTIVE": {"in_progress": 1},
		},
	}
	env := accumulator.Envelope[*whatsnext.WhatsNextLitePayload]{
		SchemaVersion:  whatsnext.MaterializedViewSchemaVersion,
		MaterializedAt: time.Now().UTC(),
		Payload:        payload,
	}
	data, err := json.MarshalIndent(env, "", "  ")
	require.NoError(t, err)
	require.NoError(t, fileutil.WriteFile(litePath, data, paths.FilePerm644))

	store := &mockWhatsNextStorage{
		data: map[string]map[string]any{
			"PRI-ACTIVE": {
				objects.FieldKeyKind:        objects.KindPriorityPlan,
				objects.FieldKeyID:          "PRI-ACTIVE",
				objects.FieldKeyTitle:       "Active In Progress Plan",
				objects.FieldKeyStatus:      objects.ObjectStatusInProgress,
				objects.FieldKeyActiveOrder: 1,
			},
		},
	}

	cmd := NewWhatsNextCmd()
	require.NoError(t, cmd.Flags().Set("skip-measure", "true"))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	initTestCmdContext(t, cmd, proj.Root, store)

	// Verify GetOrRecoverPayload hot path is fast (< 100ms) for active in-progress plan
	startPayload := time.Now()
	loaded, errPayload := whatsnext.GetOrRecoverPayload(context.Background(), store, proj.Root, 2*time.Minute)
	payloadDuration := time.Since(startPayload)
	require.NoError(t, errPayload)
	require.NotNil(t, loaded)
	if payloadDuration > 100*time.Millisecond {
		t.Errorf("GetOrRecoverPayload hot path took %v, expected <= 100ms", payloadDuration)
	}
	assert.False(t, loaded.Stale)
	assert.Equal(t, "PRI-ACTIVE", loaded.LeadPlan.ID)

	start := time.Now()
	err = runWhatsNext(cmd, nil)
	duration := time.Since(start)

	require.NoError(t, err)
	assert.Contains(t, buf.String(), "PRI-ACTIVE")
	if duration > 15*time.Second {
		t.Errorf("command took %v, expected <= 15s", duration)
	}
}

func TestRunWhatsNext_CompletedPlan_AutoRefreshesAndDoesNotReturnCompletedPlan(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	stateDir := filepath.Join(proj.Root, paths.ProjectDataDir, paths.StateDir)
	require.NoError(t, fileutil.MkdirAll(stateDir, paths.DirPerm755))

	// Pre-populate whats_next_lite.json with PRI-OLD as lead plan (stale cache)
	litePath := filepath.Join(stateDir, "whats_next_lite.json")
	payload := &whatsnext.WhatsNextLitePayload{
		SchemaVersion:  whatsnext.MaterializedViewSchemaVersion,
		MaterializedAt: time.Now().UTC(),
		LeadPlan: &whatsnext.WhatsNextPriorityPlan{
			ID:     "PRI-OLD",
			Title:  "Old Completed Plan",
			Status: objects.ObjectStatusInProgress, // cache still says in_progress
		},
		ActivePlans: []whatsnext.WhatsNextPriorityPlan{
			{
				ID:     "PRI-OLD",
				Title:  "Old Completed Plan",
				Status: objects.ObjectStatusInProgress,
			},
		},
		BacklogCountsByPlan: map[string]map[string]int{
			"PRI-OLD": {"completed": 3},
		},
	}
	env := accumulator.Envelope[*whatsnext.WhatsNextLitePayload]{
		SchemaVersion:  whatsnext.MaterializedViewSchemaVersion,
		MaterializedAt: time.Now().UTC(),
		Payload:        payload,
	}
	data, err := json.MarshalIndent(env, "", "  ")
	require.NoError(t, err)
	require.NoError(t, fileutil.WriteFile(litePath, data, paths.FilePerm644))

	// In CAS / storage: PRI-OLD is now complete! And PRI-NEXT is active/in_progress.
	store := &mockWhatsNextStorage{
		data: map[string]map[string]any{
			"PRI-OLD": {
				objects.FieldKeyKind:        objects.KindPriorityPlan,
				objects.FieldKeyID:          "PRI-OLD",
				objects.FieldKeyTitle:       "Old Completed Plan",
				objects.FieldKeyStatus:      objects.ObjectStatusComplete,
				objects.FieldKeyActiveOrder: 1,
			},
			"PRI-NEXT": {
				objects.FieldKeyKind:        objects.KindPriorityPlan,
				objects.FieldKeyID:          "PRI-NEXT",
				objects.FieldKeyTitle:       "Next Active Plan",
				objects.FieldKeyStatus:      objects.ObjectStatusInProgress,
				objects.FieldKeyActiveOrder: 2,
			},
			"BLI-NEXT-01": {
				objects.FieldKeyKind:            objects.KindBacklogItem,
				objects.FieldKeyID:              "BLI-NEXT-01",
				objects.FieldKeyTitle:           "Next BLI",
				objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
				objects.FieldKeyPriorityPlanRef: "PRI-NEXT",
			},
		},
	}

	cmd := NewWhatsNextCmd()
	require.NoError(t, cmd.Flags().Set("skip-measure", "true"))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	initTestCmdContext(t, cmd, proj.Root, store)

	// Run whats-next WITHOUT --sync-sweep
	err = runWhatsNext(cmd, nil)
	require.NoError(t, err)

	outStr := buf.String()
	// Must NOT return the completed plan PRI-OLD as Priority plan
	assert.NotContains(t, outStr, "Priority plan: PRI-OLD")
	// Must automatically refresh and return PRI-NEXT
	assert.Contains(t, outStr, "PRI-NEXT")

	// Verify that whats_next_lite.json on disk was automatically updated
	view := whatsnext.NewWhatsNextMaterializedView(proj.Root)
	freshPayload, err := view.LoadFromLiteFile()
	require.NoError(t, err)
	require.NotNil(t, freshPayload)
	if freshPayload.LeadPlan != nil {
		assert.NotEqual(t, "PRI-OLD", freshPayload.LeadPlan.ID, "disk cache should not have PRI-OLD as lead plan")
		assert.Equal(t, "PRI-NEXT", freshPayload.LeadPlan.ID, "disk cache should have PRI-NEXT as lead plan")
	}
}

func TestRunWhatsNext_CachedStatusComplete_AutoRefreshes(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	stateDir := filepath.Join(proj.Root, paths.ProjectDataDir, paths.StateDir)
	require.NoError(t, fileutil.MkdirAll(stateDir, paths.DirPerm755))

	// Pre-populate whats_next_lite.json with PRI-DONE where cached status is already "complete"
	litePath := filepath.Join(stateDir, "whats_next_lite.json")
	payload := &whatsnext.WhatsNextLitePayload{
		SchemaVersion:  whatsnext.MaterializedViewSchemaVersion,
		MaterializedAt: time.Now().UTC(),
		LeadPlan: &whatsnext.WhatsNextPriorityPlan{
			ID:     "PRI-DONE",
			Title:  "Done Plan",
			Status: objects.ObjectStatusComplete,
		},
		ActivePlans: []whatsnext.WhatsNextPriorityPlan{
			{
				ID:     "PRI-DONE",
				Title:  "Done Plan",
				Status: objects.ObjectStatusComplete,
			},
		},
	}
	env := accumulator.Envelope[*whatsnext.WhatsNextLitePayload]{
		SchemaVersion:  whatsnext.MaterializedViewSchemaVersion,
		MaterializedAt: time.Now().UTC(),
		Payload:        payload,
	}
	data, err := json.MarshalIndent(env, "", "  ")
	require.NoError(t, err)
	require.NoError(t, fileutil.WriteFile(litePath, data, paths.FilePerm644))

	store := &mockWhatsNextStorage{
		data: map[string]map[string]any{
			"PRI-DONE": {
				objects.FieldKeyKind:        objects.KindPriorityPlan,
				objects.FieldKeyID:          "PRI-DONE",
				objects.FieldKeyTitle:       "Done Plan",
				objects.FieldKeyStatus:      objects.ObjectStatusComplete,
				objects.FieldKeyActiveOrder: 1,
			},
			"PRI-FRESH": {
				objects.FieldKeyKind:        objects.KindPriorityPlan,
				objects.FieldKeyID:          "PRI-FRESH",
				objects.FieldKeyTitle:       "Fresh Plan",
				objects.FieldKeyStatus:      objects.ObjectStatusInProgress,
				objects.FieldKeyActiveOrder: 2,
			},
		},
	}

	cmd := NewWhatsNextCmd()
	require.NoError(t, cmd.Flags().Set("skip-measure", "true"))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	initTestCmdContext(t, cmd, proj.Root, store)

	err = runWhatsNext(cmd, nil)
	require.NoError(t, err)

	outStr := buf.String()
	assert.NotContains(t, outStr, "Priority plan: PRI-DONE")
	assert.Contains(t, outStr, "PRI-FRESH")
}

func TestRunWhatsNext_HigherActiveOrderPromotion_AutoRefreshes(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	stateDir := filepath.Join(proj.Root, paths.ProjectDataDir, paths.StateDir)
	require.NoError(t, fileutil.MkdirAll(stateDir, paths.DirPerm755))

	// Pre-populate whats_next_lite.json with PRI-LOWER (active_order: 10) as lead plan
	litePath := filepath.Join(stateDir, "whats_next_lite.json")
	payload := &whatsnext.WhatsNextLitePayload{
		SchemaVersion:  whatsnext.MaterializedViewSchemaVersion,
		MaterializedAt: time.Now().UTC(),
		LeadPlan: &whatsnext.WhatsNextPriorityPlan{
			ID:     "PRI-LOWER",
			Title:  "Lower Priority Plan",
			Status: objects.ObjectStatusActive,
		},
		ActivePlans: []whatsnext.WhatsNextPriorityPlan{
			{
				ID:     "PRI-LOWER",
				Title:  "Lower Priority Plan",
				Status: objects.ObjectStatusActive,
			},
		},
	}
	env := accumulator.Envelope[*whatsnext.WhatsNextLitePayload]{
		SchemaVersion:  whatsnext.MaterializedViewSchemaVersion,
		MaterializedAt: time.Now().UTC(),
		Payload:        payload,
	}
	data, err := json.MarshalIndent(env, "", "  ")
	require.NoError(t, err)
	require.NoError(t, fileutil.WriteFile(litePath, data, paths.FilePerm644))

	// In CAS storage: PRI-HIGHER was promoted with active_order=1, superseding PRI-LOWER (active_order=10)
	store := &mockWhatsNextStorage{
		data: map[string]map[string]any{
			"PRI-LOWER": {
				objects.FieldKeyKind:        objects.KindPriorityPlan,
				objects.FieldKeyID:          "PRI-LOWER",
				objects.FieldKeyTitle:       "Lower Priority Plan",
				objects.FieldKeyStatus:      objects.ObjectStatusActive,
				objects.FieldKeyActiveOrder: 10,
			},
			"PRI-HIGHER": {
				objects.FieldKeyKind:        objects.KindPriorityPlan,
				objects.FieldKeyID:          "PRI-HIGHER",
				objects.FieldKeyTitle:       "Higher Priority Plan",
				objects.FieldKeyStatus:      objects.ObjectStatusActive,
				objects.FieldKeyActiveOrder: 1,
			},
			"BLI-HIGH-1": {
				objects.FieldKeyKind:            objects.KindBacklogItem,
				objects.FieldKeyID:              "BLI-HIGH-1",
				objects.FieldKeyTitle:           "Top BLI",
				objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
				objects.FieldKeyPriorityPlanRef: "PRI-HIGHER",
			},
		},
	}

	cmd := NewWhatsNextCmd()
	require.NoError(t, cmd.Flags().Set("skip-measure", "true"))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	initTestCmdContext(t, cmd, proj.Root, store)

	// Execute whats-next on hot-path (WITHOUT --sync-sweep)
	err = runWhatsNext(cmd, nil)
	require.NoError(t, err)

	outStr := buf.String()
	assert.NotContains(t, outStr, "Priority plan: PRI-LOWER")
	assert.Contains(t, outStr, "PRI-HIGHER")

	// Verify disk cache was automatically updated to PRI-HIGHER
	view := whatsnext.NewWhatsNextMaterializedView(proj.Root)
	freshPayload, err := view.LoadFromLiteFile()
	require.NoError(t, err)
	require.NotNil(t, freshPayload)
	require.NotNil(t, freshPayload.LeadPlan)
	assert.Equal(t, "PRI-HIGHER", freshPayload.LeadPlan.ID)
}

func TestRunWhatsNext_CorruptedCacheFallback(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	stateDir := filepath.Join(proj.Root, paths.ProjectDataDir, paths.StateDir)
	require.NoError(t, fileutil.MkdirAll(stateDir, paths.DirPerm755))

	// Write malformed/corrupted JSON to whats_next_lite.json
	litePath := filepath.Join(stateDir, "whats_next_lite.json")
	corruptedData := []byte(`{"schema_version": "whats_next_lite_v1", "materialized_at": "invalid-time", "lead_plan": {broken_json`)
	require.NoError(t, fileutil.WriteFile(litePath, corruptedData, paths.FilePerm644))

	store := &mockWhatsNextStorage{
		data: map[string]map[string]any{
			"PRI-RESILIENT": {
				objects.FieldKeyKind:        objects.KindPriorityPlan,
				objects.FieldKeyID:          "PRI-RESILIENT",
				objects.FieldKeyTitle:       "Resilient Live Plan",
				objects.FieldKeyStatus:      objects.ObjectStatusInProgress,
				objects.FieldKeyActiveOrder: 1,
			},
			"BLI-RES-1": {
				objects.FieldKeyKind:            objects.KindBacklogItem,
				objects.FieldKeyID:              "BLI-RES-1",
				objects.FieldKeyTitle:           "Live BLI",
				objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
				objects.FieldKeyPriorityPlanRef: "PRI-RESILIENT",
			},
		},
	}

	cmd := NewWhatsNextCmd()
	require.NoError(t, cmd.Flags().Set("skip-measure", "true"))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	initTestCmdContext(t, cmd, proj.Root, store)

	// Must not panic or return error despite corrupted cache
	err := runWhatsNext(cmd, nil)
	require.NoError(t, err)

	outStr := buf.String()
	assert.Contains(t, outStr, "PRI-RESILIENT")

	// Disk cache must be repaired with valid JSON
	view := whatsnext.NewWhatsNextMaterializedView(proj.Root)
	repairedPayload, err := view.LoadFromLiteFile()
	require.NoError(t, err, "corrupted lite file should have been repaired")
	require.NotNil(t, repairedPayload)
	if repairedPayload.LeadPlan != nil {
		assert.Equal(t, "PRI-RESILIENT", repairedPayload.LeadPlan.ID)
	}
}

func TestRunWhatsNext_PermissionDeniedFallback(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	stateDir := filepath.Join(proj.Root, paths.ProjectDataDir, paths.StateDir)
	require.NoError(t, fileutil.MkdirAll(stateDir, paths.DirPerm755))

	litePath := filepath.Join(stateDir, "whats_next_lite.json")
	require.NoError(t, fileutil.WriteFile(litePath, []byte("{}"), paths.FilePerm644))

	// Remove read permissions from the cache file
	require.NoError(t, os.Chmod(litePath, 0000))
	t.Cleanup(func() {
		_ = os.Chmod(litePath, 0644)
	})

	store := &mockWhatsNextStorage{
		data: map[string]map[string]any{
			"PRI-PERM": {
				objects.FieldKeyKind:        objects.KindPriorityPlan,
				objects.FieldKeyID:          "PRI-PERM",
				objects.FieldKeyTitle:       "Permission Test Plan",
				objects.FieldKeyStatus:      objects.ObjectStatusInProgress,
				objects.FieldKeyActiveOrder: 1,
			},
		},
	}

	cmd := NewWhatsNextCmd()
	require.NoError(t, cmd.Flags().Set("skip-measure", "true"))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	initTestCmdContext(t, cmd, proj.Root, store)

	// Graceful execution without panic or hard error
	err := runWhatsNext(cmd, nil)
	require.NoError(t, err)

	outStr := buf.String()
	assert.Contains(t, outStr, "PRI-PERM")
}

func TestRunWhatsNext_EmptyStorage_CorruptedCacheGraceful(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	stateDir := filepath.Join(proj.Root, paths.ProjectDataDir, paths.StateDir)
	require.NoError(t, fileutil.MkdirAll(stateDir, paths.DirPerm755))

	litePath := filepath.Join(stateDir, "whats_next_lite.json")
	require.NoError(t, fileutil.WriteFile(litePath, []byte("NOT-JSON-AT-ALL"), paths.FilePerm644))

	store := &mockWhatsNextStorage{
		data: map[string]map[string]any{},
	}

	cmd := NewWhatsNextCmd()
	require.NoError(t, cmd.Flags().Set("skip-measure", "true"))
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	initTestCmdContext(t, cmd, proj.Root, store)

	err := runWhatsNext(cmd, nil)
	require.NoError(t, err)
}
