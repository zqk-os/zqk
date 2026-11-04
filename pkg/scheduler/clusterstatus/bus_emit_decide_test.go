package clusterstatus

import (
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestBus_NewBus_DefaultAndCustomTTL(t *testing.T) {
	tempDir := t.TempDir()

	// TTL <= 0 should set DefaultTTL
	busDefault := NewBus(tempDir, 0)
	require.NotNil(t, busDefault)
	assert.Equal(t, DefaultTTL, busDefault.ttl)

	busNegative := NewBus(tempDir, -5*time.Second)
	require.NotNil(t, busNegative)
	assert.Equal(t, DefaultTTL, busNegative.ttl)

	// Custom positive TTL
	customTTL := 45 * time.Second
	busCustom := NewBus(tempDir, customTTL)
	require.NotNil(t, busCustom)
	assert.Equal(t, customTTL, busCustom.ttl)
}

func TestBus_Emit_ZeroAndExplicitTimestamp(t *testing.T) {
	tempDir := t.TempDir()
	bus := NewBus(tempDir, time.Minute)

	// Emit with zero UpdatedAt
	evt1 := Event{
		NodeID: "node-1",
		RootID: "root-1",
		JobID:  "job-1",
		TaskID: "task-1",
		Phase:  PhaseRunning,
	}
	err1 := bus.Emit(evt1)
	require.NoError(t, err1)

	latest1, phase1, errLatest1 := bus.Latest(Watch{RootID: "root-1", JobID: "job-1", TaskID: "task-1"})
	require.NoError(t, errLatest1)
	assert.Equal(t, PhaseRunning, phase1)
	assert.False(t, latest1.UpdatedAt.IsZero())

	// Emit with explicit UpdatedAt inside freshness window (Bus TTL is time.Minute).
	// A fixed wall calendar instant ages into PhaseStale as CI clocks move.
	explicitTime := time.Now().UTC().Add(-2 * time.Second).Truncate(time.Millisecond)
	evt2 := Event{
		NodeID:    "node-2",
		RootID:    "root-2",
		JobID:     "job-2",
		TaskID:    "task-2",
		Phase:     PhaseSucceeded,
		Progress:  1.0,
		UpdatedAt: explicitTime,
	}
	err2 := bus.Emit(evt2)
	require.NoError(t, err2)

	latest2, phase2, errLatest2 := bus.Latest(Watch{RootID: "root-2", JobID: "job-2", TaskID: "task-2"})
	require.NoError(t, errLatest2)
	assert.Equal(t, PhaseSucceeded, phase2)
	assert.Equal(t, explicitTime, latest2.UpdatedAt)

	// Verify events are persisted in JSONL log file
	data, errRead := fileutil.ReadFile(bus.path)
	require.NoError(t, errRead)
	assert.Contains(t, string(data), "node-1")
	assert.Contains(t, string(data), "node-2")
}

func TestBus_Emit_FileErrorHandling(t *testing.T) {
	tempDir := t.TempDir()

	// Create a regular file where the directory should be
	blockingFile := filepath.Join(tempDir, ".zqk")
	require.NoError(t, fileutil.WriteFile(blockingFile, []byte("blocking"), 0644))

	busErr := NewBus(tempDir, time.Minute)
	err := busErr.Emit(Event{
		NodeID: "n-err",
		RootID: "r-err",
		JobID:  "j-err",
		Phase:  PhaseRunning,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cluster status log dir")

	// Test open file failure when directory is read-only
	tempDir2 := t.TempDir()
	bus2 := NewBus(tempDir2, time.Minute)
	logDir := filepath.Dir(bus2.path)
	require.NoError(t, fileutil.MkdirAll(logDir, 0755))
	// Create log path as a directory so OpenFile fails
	require.NoError(t, fileutil.MkdirAll(bus2.path, 0755))

	errOpen := bus2.Emit(Event{
		NodeID: "n-open-err",
		RootID: "r-open-err",
		JobID:  "j-open-err",
		Phase:  PhaseRunning,
	})
	require.Error(t, errOpen)
	assert.Contains(t, errOpen.Error(), "open cluster status log")
}

func TestBus_Latest_Branches(t *testing.T) {
	tempDir := t.TempDir()
	bus := NewBus(tempDir, 100*time.Millisecond)

	// 1. Key not present in latest map
	watchMissing := Watch{RootID: "missing-root", JobID: "missing-job", TaskID: "missing-task"}
	evtMissing, phaseMissing, errMissing := bus.Latest(watchMissing)
	require.NoError(t, errMissing)
	assert.Equal(t, PhaseStale, phaseMissing)
	assert.Equal(t, PhaseStale, evtMissing.Phase)
	assert.Equal(t, "missing-root", evtMissing.RootID)
	assert.Equal(t, "missing-job", evtMissing.JobID)
	assert.Equal(t, "missing-task", evtMissing.TaskID)

	// 2. Key present and within TTL
	now := time.Now().UTC()
	evtFresh := Event{
		RootID:    "r-fresh",
		JobID:     "j-fresh",
		TaskID:    "t-fresh",
		Phase:     PhaseRunning,
		UpdatedAt: now,
	}
	require.NoError(t, bus.Emit(evtFresh))

	evtGot, phaseGot, errGot := bus.Latest(Watch{RootID: "r-fresh", JobID: "j-fresh", TaskID: "t-fresh"})
	require.NoError(t, errGot)
	assert.Equal(t, PhaseRunning, phaseGot)
	assert.Equal(t, PhaseRunning, evtGot.Phase)

	// 3. Key present but expired past custom Watch TTL
	evtShort := Event{
		RootID:    "r-short",
		JobID:     "j-short",
		TaskID:    "t-short",
		Phase:     PhaseSucceeded,
		UpdatedAt: now.Add(-500 * time.Millisecond),
	}
	require.NoError(t, bus.Emit(evtShort))

	evtExpired, phaseExpired, errExpired := bus.Latest(Watch{
		RootID: "r-short",
		JobID:  "j-short",
		TaskID: "t-short",
		TTL:    10 * time.Millisecond,
	})
	require.NoError(t, errExpired)
	assert.Equal(t, PhaseStale, phaseExpired)
	assert.Equal(t, PhaseStale, evtExpired.Phase)
}

func TestBus_Decide_AllPhases(t *testing.T) {
	tempDir := t.TempDir()
	bus := NewBus(tempDir, time.Hour)

	cases := []struct {
		name         string
		event        Event
		expectedPass bool
		expectedDiff string
		expectedPh   string
	}{
		{
			name: "succeeded phase allows forward",
			event: Event{
				RootID: "r-succ",
				JobID:  "j-succ",
				Phase:  PhaseSucceeded,
			},
			expectedPass: true,
			expectedDiff: "peer succeeded",
			expectedPh:   PhaseSucceeded,
		},
		{
			name: "running phase blocks forward",
			event: Event{
				RootID: "r-run",
				JobID:  "j-run",
				Phase:  PhaseRunning,
			},
			expectedPass: false,
			expectedDiff: "peer still running",
			expectedPh:   PhaseRunning,
		},
		{
			name: "failed phase blocks forward with error",
			event: Event{
				RootID: "r-fail",
				JobID:  "j-fail",
				Phase:  PhaseFailed,
				Error:  "connection reset by peer",
			},
			expectedPass: false,
			expectedDiff: "peer failed: connection reset by peer",
			expectedPh:   PhaseFailed,
		},
		{
			name: "stale phase blocks forward fail-closed",
			event: Event{
				RootID: "r-stale",
				JobID:  "j-stale",
				Phase:  PhaseStale,
			},
			expectedPass: false,
			expectedDiff: "peer status stale (fail closed)",
			expectedPh:   PhaseStale,
		},
		{
			name: "degraded phase blocks forward fail-closed",
			event: Event{
				RootID: "r-deg",
				JobID:  "j-deg",
				Phase:  PhaseDegraded,
			},
			expectedPass: false,
			expectedDiff: "peer status degraded (fail closed)",
			expectedPh:   PhaseDegraded,
		},
		{
			name: "unknown phase blocks forward fail-closed",
			event: Event{
				RootID: "r-unk",
				JobID:  "j-unk",
				Phase:  "mysterious_phase_xyz",
			},
			expectedPass: false,
			expectedDiff: "unknown peer phase",
			expectedPh:   "mysterious_phase_xyz",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, bus.Emit(tc.event))
			decision := bus.Decide(Watch{RootID: tc.event.RootID, JobID: tc.event.JobID})
			assert.Equal(t, tc.expectedPass, decision.AllowForward)
			assert.Equal(t, tc.expectedDiff, decision.Reason)
			assert.Equal(t, tc.expectedPh, decision.Phase)
		})
	}
}

func TestBus_Decide_MissingWatchReturnsStaleFailClosed(t *testing.T) {
	tempDir := t.TempDir()
	bus := NewBus(tempDir, time.Hour)

	decision := bus.Decide(Watch{RootID: "non-existent-root", JobID: "non-existent-job"})
	assert.False(t, decision.AllowForward)
	assert.Equal(t, PhaseStale, decision.Phase)
	assert.Equal(t, "peer status stale (fail closed)", decision.Reason)
}

func TestBus_ConcurrentEmitAndDecide(t *testing.T) {
	tempDir := t.TempDir()
	bus := NewBus(tempDir, time.Second)

	var wg sync.WaitGroup
	workers := 10
	iterations := 20

	for i := 0; i < workers; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("test_bus_concurrent", "concurrent emit and decide").StartSimple(func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				evt := Event{
					NodeID:   "node",
					RootID:   "concurrent-root",
					JobID:    "job",
					TaskID:   "task",
					Phase:    PhaseRunning,
					Progress: float64(j) / float64(iterations),
				}
				if j == iterations-1 {
					evt.Phase = PhaseSucceeded
				}
				_ = bus.Emit(evt)
				_ = bus.Decide(Watch{RootID: "concurrent-root", JobID: "job", TaskID: "task"})
			}
		})
	}

	wg.Wait()

	dec := bus.Decide(Watch{RootID: "concurrent-root", JobID: "job", TaskID: "task"})
	assert.NotEmpty(t, dec.Phase)
}

func TestEventJSONSerialization(t *testing.T) {
	now := time.Date(2026, 11, 4, 1, 2, 3, 0, time.UTC)
	evt := Event{
		NodeID:    "node-abc",
		RootID:    "root-def",
		JobID:     "job-123",
		TaskID:    "task-456",
		Phase:     PhaseRunning,
		Progress:  0.75,
		Error:     "transient network blip",
		UpdatedAt: now,
	}

	bytes, err := json.Marshal(evt)
	require.NoError(t, err)

	var parsed Event
	require.NoError(t, json.Unmarshal(bytes, &parsed))
	assert.Equal(t, evt.NodeID, parsed.NodeID)
	assert.Equal(t, evt.RootID, parsed.RootID)
	assert.Equal(t, evt.JobID, parsed.JobID)
	assert.Equal(t, evt.TaskID, parsed.TaskID)
	assert.Equal(t, evt.Phase, parsed.Phase)
	assert.Equal(t, evt.Progress, parsed.Progress)
	assert.Equal(t, evt.Error, parsed.Error)
	assert.Equal(t, evt.UpdatedAt, parsed.UpdatedAt)
}
