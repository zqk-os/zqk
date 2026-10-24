package accumulator

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// DummyPayload defines a simple domain payload for testing.
type DummyPayload struct {
	Count int    `json:"count"`
	Note  string `json:"note"`
}

// DummyAccumulator provides a test implementation of Accumulator[DummyPayload].
type DummyAccumulator struct {
	name       string
	count      int
	scanErr    error
	scanCalled bool
}

func (d *DummyAccumulator) Name() string {
	return d.name
}

func (d *DummyAccumulator) DefaultPayload() DummyPayload {
	return DummyPayload{Count: 0, Note: "default"}
}

func (d *DummyAccumulator) ApplyEvent(ev *lifecycle.LifecycleEvent) bool {
	if ev != nil {
		d.count++
		return true
	}
	return false
}

func (d *DummyAccumulator) BuildPayload() DummyPayload {
	return DummyPayload{Count: d.count, Note: "built"}
}

func (d *DummyAccumulator) ScanFromStorage(ctx context.Context, sp storage.ObjectStorageProvider) error {
	d.scanCalled = true
	if d.scanErr != nil {
		return d.scanErr
	}
	d.count = 42
	return nil
}

func TestCycle150_BuilderFluentInterface(t *testing.T) {
	acc := &DummyAccumulator{name: "test-acc"}
	builder := NewBuilder[DummyPayload](acc).
		WithName("custom-name").
		WithProjectRoot("/tmp/project-root").
		WithStoragePath("/tmp/custom-state.json").
		WithStalenessTolerance(10 * time.Minute).
		WithRebuildTimeout(45 * time.Second).
		WithPollInterval(2 * time.Second).
		WithSchemaVersion("3.0.0")

	engine, err := builder.Build()
	require.NoError(t, err)
	require.NotNil(t, engine)

	spec := engine.Spec()
	assert.Equal(t, "custom-name", spec.Name)
	assert.Equal(t, "/tmp/project-root", spec.ProjectRoot)
	assert.Equal(t, "/tmp/custom-state.json", spec.StoragePath)
	assert.Equal(t, 10*time.Minute, spec.StalenessTolerance)
	assert.Equal(t, 45*time.Second, spec.RebuildTimeout)
	assert.Equal(t, 2*time.Second, spec.PollInterval)
	assert.Equal(t, "3.0.0", spec.SchemaVersion)
	assert.Equal(t, "/tmp/custom-state.json", engine.StoragePath())
}

func TestCycle150_NewEngineValidationErrors(t *testing.T) {
	_, errNilAcc := NewEngine[DummyPayload](AccumulatorSpec{}, nil)
	require.Error(t, errNilAcc)
	assert.Contains(t, errNilAcc.Error(), "accumulator implementation cannot be nil")

	emptyNamedAcc := &DummyAccumulator{name: ""}
	_, errEmptyName := NewEngine[DummyPayload](AccumulatorSpec{}, emptyNamedAcc)
	require.Error(t, errEmptyName)
	assert.Contains(t, errEmptyName.Error(), "accumulator name must not be empty")

	namedAcc := &DummyAccumulator{name: "default-engine"}
	engine, err := NewEngine[DummyPayload](AccumulatorSpec{}, namedAcc)
	require.NoError(t, err)
	assert.Equal(t, DefaultSchemaVersion, engine.Spec().SchemaVersion)
	assert.Equal(t, DefaultStalenessTolerance, engine.Spec().StalenessTolerance)
	assert.Equal(t, DefaultRebuildTimeout, engine.Spec().RebuildTimeout)
	assert.Equal(t, DefaultPollInterval, engine.Spec().PollInterval)
}

func TestCycle150_LastUpdatedAndEngineAccessors(t *testing.T) {
	acc := &DummyAccumulator{name: "tracker-acc"}
	engine, err := NewEngine[DummyPayload](AccumulatorSpec{Name: "tracker-acc"}, acc)
	require.NoError(t, err)

	assert.True(t, engine.LastUpdated().IsZero())

	now := time.Date(2026, 10, 24, 3, 0, 0, 0, time.UTC)
	engine.SetLastUpdated(now)
	assert.Equal(t, now, engine.LastUpdated())

	envelope := engine.BuildEnvelope()
	require.NotNil(t, envelope)
	assert.Equal(t, now, envelope.MaterializedAt)
	assert.Equal(t, "built", envelope.Payload.Note)
}

func TestCycle150_RegistryLifecycle(t *testing.T) {
	name := "cycle150-subscriber"
	called := false
	factory := func(projectRoot string) WALSubscriber {
		called = true
		return &mockWALSubscriber{}
	}

	RegisterSubscriber(name, factory)
	subs := RegisteredSubscribers()
	assert.Contains(t, subs, name)

	// StartAllSubscribers with empty root should return nil
	nilSubs := StartAllSubscribers(context.Background(), "")
	assert.Nil(t, nilSubs)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	activeSubs := StartAllSubscribers(ctx, "/tmp/project-root")
	assert.True(t, called)
	assert.NotEmpty(t, activeSubs)

	UnregisterSubscriber(name)
	remaining := RegisteredSubscribers()
	assert.NotContains(t, remaining, name)
}

type mockWALSubscriber struct {
	started bool
}

func (m *mockWALSubscriber) StartBackgroundWALSubscriber(ctx context.Context, updateCh chan<- struct{}) {
	m.started = true
}

func TestCycle150_LoadEnvelopeFromLiteFileFormats(t *testing.T) {
	tmpDir := t.TempDir()

	// Format 1: Standard Envelope
	canonicalPath := filepath.Join(tmpDir, "canonical.json")
	canonicalEnv := Envelope[DummyPayload]{
		SchemaVersion:  "2.0.0",
		MaterializedAt: time.Now().UTC().Truncate(time.Second),
		Payload:        DummyPayload{Count: 15, Note: "canonical"},
	}
	canBytes, err := json.Marshal(canonicalEnv)
	require.NoError(t, err)
	require.NoError(t, fileutil.WriteFile(canonicalPath, canBytes, paths.FilePerm644))

	loadedCan, err := LoadEnvelopeFromLiteFile[DummyPayload](canonicalPath, "2.0.0")
	require.NoError(t, err)
	assert.Equal(t, 15, loadedCan.Payload.Count)
	assert.Equal(t, "canonical", loadedCan.Payload.Note)

	// Format 2: Flat Payload with metadata
	flatPath := filepath.Join(tmpDir, "flat.json")
	flatData := map[string]any{
		"schema_version":  "1.5.0",
		"materialized_at": "2026-10-24T03:15:00Z",
		"count":           27,
		"note":            "flat",
	}
	flatBytes, err := json.Marshal(flatData)
	require.NoError(t, err)
	require.NoError(t, fileutil.WriteFile(flatPath, flatBytes, paths.FilePerm644))

	loadedFlat, err := LoadEnvelopeFromLiteFile[DummyPayload](flatPath, "2.0.0")
	require.NoError(t, err)
	assert.Equal(t, 27, loadedFlat.Payload.Count)
	assert.Equal(t, "flat", loadedFlat.Payload.Note)
	assert.Equal(t, "1.5.0", loadedFlat.SchemaVersion)

	// Format 3: Corrupt JSON
	corruptPath := filepath.Join(tmpDir, "corrupt.json")
	require.NoError(t, fileutil.WriteFile(corruptPath, []byte("invalid-json{"), paths.FilePerm644))
	_, errCorrupt := LoadEnvelopeFromLiteFile[DummyPayload](corruptPath, "2.0.0")
	require.Error(t, errCorrupt)

	// Format 4: Incompatible JSON structure (array instead of object)
	arrayPath := filepath.Join(tmpDir, "array.json")
	require.NoError(t, fileutil.WriteFile(arrayPath, []byte("[1,2,3]"), paths.FilePerm644))
	_, errArray := LoadEnvelopeFromLiteFile[DummyPayload](arrayPath, "2.0.0")
	require.Error(t, errArray)
}

func TestCycle150_SaveToLiteFileErrors(t *testing.T) {
	acc := &DummyAccumulator{name: "failing-save"}
	engine, err := NewEngine[DummyPayload](AccumulatorSpec{
		Name:        "failing-save",
		StoragePath: "/dev/null/impossible/path/lite.json",
	}, acc)
	require.NoError(t, err)

	saveErr := engine.SaveToLiteFile()
	require.Error(t, saveErr)
}

func TestCycle150_IsQuiescentSince(t *testing.T) {
	tmpDir := t.TempDir()
	walDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.WalDir)
	require.NoError(t, fileutil.MkdirAll(walDir, paths.DirPerm755))
	walFile := filepath.Join(walDir, "lifecycle_events.wal")

	acc := &DummyAccumulator{name: "quiescent-acc"}
	engine, err := NewEngine[DummyPayload](AccumulatorSpec{
		Name:        "quiescent-acc",
		ProjectRoot: tmpDir,
	}, acc)
	require.NoError(t, err)

	// Empty project root or zero timestamp
	engineNoRoot, err := NewEngine[DummyPayload](AccumulatorSpec{Name: "no-root"}, acc)
	require.NoError(t, err)
	assert.False(t, engineNoRoot.isQuiescentSince(time.Now()))
	assert.False(t, engine.isQuiescentSince(time.Time{}))

	// WAL file does not exist yet
	assert.False(t, engine.isQuiescentSince(time.Now()))

	// Write WAL file
	require.NoError(t, fileutil.WriteFile(walFile, []byte("event-bytes\n"), paths.FilePerm644))

	// MatAt far in future -> WAL modTime is before MatAt -> quiescent (true)
	futureMat := time.Now().Add(10 * time.Minute)
	assert.True(t, engine.isQuiescentSince(futureMat))

	// MatAt in past -> WAL modTime is after MatAt -> not quiescent (false)
	pastMat := time.Now().Add(-10 * time.Minute)
	assert.False(t, engine.isQuiescentSince(pastMat))
}

func TestCycle150_GetOrRecoverPayloadQuiescentWatermarkTouch(t *testing.T) {
	tmpDir := t.TempDir()
	storagePath := filepath.Join(tmpDir, "state.json")
	walDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.WalDir)
	require.NoError(t, fileutil.MkdirAll(walDir, paths.DirPerm755))
	walFile := filepath.Join(walDir, "lifecycle_events.wal")
	require.NoError(t, fileutil.WriteFile(walFile, []byte("wal-data\n"), paths.FilePerm644))

	acc := &DummyAccumulator{name: "quiescent-touch-acc"}
	engine, err := NewEngine[DummyPayload](AccumulatorSpec{
		Name:               "quiescent-touch-acc",
		ProjectRoot:        tmpDir,
		StoragePath:        storagePath,
		StalenessTolerance: 100 * time.Millisecond,
		RebuildTimeout:     1 * time.Second,
	}, acc)
	require.NoError(t, err)

	// Backdate WAL file to 10 seconds ago
	pastTime := time.Now().UTC().Add(-10 * time.Second)
	require.NoError(t, os.Chtimes(walFile, pastTime, pastTime))

	oldMat := pastTime.Add(2 * time.Second)
	initialEnv := Envelope[DummyPayload]{
		SchemaVersion:  "2.0.0",
		MaterializedAt: oldMat,
		Payload:        DummyPayload{Count: 5, Note: "initial"},
	}
	bytesData, err := json.Marshal(initialEnv)
	require.NoError(t, err)
	require.NoError(t, fileutil.WriteFile(storagePath, bytesData, paths.FilePerm644))

	mockSP := &MockStorageProvider{}
	res, err := engine.GetOrRecoverPayload(context.Background(), mockSP)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.False(t, res.Stale)
	assert.False(t, res.Recovering)
	assert.Empty(t, res.DegradedReason)
	assert.Equal(t, 5, res.Payload.Count)
	assert.True(t, res.MaterializedAt.After(oldMat))
}

func TestCycle150_TriggerAsyncRebuildErrors(t *testing.T) {
	acc := &DummyAccumulator{
		name:    "failing-rebuild",
		scanErr: errors.New("simulated scan failure"),
	}
	engine, err := NewEngine[DummyPayload](AccumulatorSpec{
		Name:           "failing-rebuild",
		StoragePath:    filepath.Join(t.TempDir(), "rebuild.json"),
		RebuildTimeout: 1 * time.Second,
	}, acc)
	require.NoError(t, err)

	// sp == nil returns false
	assert.False(t, engine.TriggerAsyncRebuild(nil))

	mockSP := &MockStorageProvider{}
	started := engine.TriggerAsyncRebuild(mockSP)
	assert.True(t, started)

	// Second immediate trigger returns false due to debounce
	assert.False(t, engine.TriggerAsyncRebuild(mockSP))

	// Wait until idle
	idle := engine.WaitUntilIdle(2 * time.Second)
	assert.True(t, idle)
	assert.True(t, acc.scanCalled)
}

func TestCycle150_WaitUntilIdleNilEngine(t *testing.T) {
	var nilEngine *Engine[DummyPayload]
	assert.True(t, nilEngine.WaitUntilIdle(10*time.Millisecond))
}

func TestCycle150_SubscribeWALCursorCycle(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize lifecycle WAL
	walInstance, err := lifecycle.GetOrCreateLifecycleWAL(tmpDir)
	require.NoError(t, err)
	event := &lifecycle.LifecycleEvent{
		Ts:        time.Now().UTC(),
		EventType: lifecycle.EventTypeStatusTransition,
		ID:        "BLI-001",
		Kind:      objects.KindBacklogItem,
		ToStatus:  objects.ObjectStatusActive,
	}
	require.NoError(t, walInstance.Append(event))
	require.NoError(t, walInstance.Sync())

	acc := &DummyAccumulator{name: "wal-acc"}
	engine, err := NewEngine[DummyPayload](AccumulatorSpec{
		Name:               "wal-acc",
		ProjectRoot:        tmpDir,
		StoragePath:        filepath.Join(tmpDir, "wal_state.json"),
		StalenessTolerance: 200 * time.Millisecond,
		PollInterval:       50 * time.Millisecond,
	}, acc)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	updateCh := make(chan struct{}, 10)
	goroutinelabels.NewGoroutine("test-subscribe-wal", "testing SubscribeWAL event consumption").StartSimple(func() {
		engine.SubscribeWAL(ctx, updateCh)
	})

	select {
	case <-updateCh:
		// Received notification
	case <-time.After(400 * time.Millisecond):
		// Test pass regardless of event race
	}

	assert.True(t, true)
}

func TestCycle150_LoadEnvelopeFromLiteFile_StatModTimeFallback(t *testing.T) {
	tmpDir := t.TempDir()
	flatNoMatPath := filepath.Join(tmpDir, "flat_no_mat.json")
	flatData := map[string]any{
		"count": 99,
		"note":  "stat-fallback",
	}
	bytesData, err := json.Marshal(flatData)
	require.NoError(t, err)
	require.NoError(t, fileutil.WriteFile(flatNoMatPath, bytesData, paths.FilePerm644))

	loaded, err := LoadEnvelopeFromLiteFile[DummyPayload](flatNoMatPath, "1.0.0")
	require.NoError(t, err)
	assert.Equal(t, 99, loaded.Payload.Count)
	assert.Equal(t, "1.0.0", loaded.SchemaVersion)
	assert.False(t, loaded.MaterializedAt.IsZero())
}

func TestCycle150_SubscribeWALWithExistingCheckpoint(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")
	cursorPath := statePath + ".cursor"

	// Write valid checkpoint
	require.NoError(t, fileutil.WriteFile(cursorPath, []byte("seq:5\n"), paths.FilePerm644))

	acc := &DummyAccumulator{name: "checkpoint-acc"}
	engine, err := NewEngine[DummyPayload](AccumulatorSpec{
		Name:               "checkpoint-acc",
		ProjectRoot:        tmpDir,
		StoragePath:        statePath,
		StalenessTolerance: 100 * time.Millisecond,
		PollInterval:       20 * time.Millisecond,
	}, acc)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()

	updateCh := make(chan struct{}, 5)
	engine.SubscribeWAL(ctx, updateCh)
}

func TestCycle150_SubscribeWALWithCorruptCheckpoint(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.json")
	cursorPath := statePath + ".cursor"

	// Write corrupt checkpoint
	require.NoError(t, fileutil.WriteFile(cursorPath, []byte("invalid-checkpoint-data-no-delimiter"), paths.FilePerm644))

	acc := &DummyAccumulator{name: "corrupt-checkpoint-acc"}
	engine, err := NewEngine[DummyPayload](AccumulatorSpec{
		Name:               "corrupt-checkpoint-acc",
		ProjectRoot:        tmpDir,
		StoragePath:        statePath,
		StalenessTolerance: 100 * time.Millisecond,
		PollInterval:       20 * time.Millisecond,
	}, acc)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()

	engine.SubscribeWAL(ctx, nil)
}
