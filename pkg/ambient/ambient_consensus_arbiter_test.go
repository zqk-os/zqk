package ambient

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type fakePruner struct{}

func (f *fakePruner) PruneMergedBranches(ctx context.Context, dryRun bool) ([]string, error) {
	return []string{"merged-feature-branch"}, nil
}

func TestAmbientCycle32_TelemetryElevation(t *testing.T) {
	now := time.Now().UTC()
	report := NewAmbientTelemetryReport(
		WithStatus("healthy"),
		WithUptime(10*time.Minute),
		WithActiveHolonsCount(3),
		WithConsensusLeasesActive(5),
		WithConsensusCommitted(12),
		WithConsensusRevoked(2),
		WithReflexActionsTotal(42),
		WithReflexRealignCount(7),
		WithRunwayWatermark(95),
		WithCircuitBreakerStatus("closed"),
		WithTimestamp(now),
		WithWarnings("low disk space", "rate limit approaching"),
	)

	assert.Equal(t, 5, report.GetConsensusLeasesActive())
	assert.Equal(t, uint64(12), report.GetConsensusCommitted())
	assert.Equal(t, uint64(2), report.GetConsensusRevoked())
	assert.Equal(t, uint64(42), report.GetReflexActionsTotal())
	assert.Equal(t, uint64(7), report.GetReflexRealignCount())
	assert.Equal(t, int64(95), report.GetRunwayWatermark())
	assert.Equal(t, "closed", report.GetCircuitBreakerStatus())
	assert.Len(t, report.GetWarnings(), 2)

	report.AddWarning("third warning")
	assert.Len(t, report.GetWarnings(), 3)

	metrics := report.GetMeshMetrics()
	assert.Equal(t, 5, metrics.ConsensusLeasesActive)
}

func TestAmbientCycle32_BackpressureRegulatorElevation(t *testing.T) {
	cfg := DefaultBackpressureConfig()
	reg, err := NewBackpressureRegulator(cfg)
	require.NoError(t, err)
	defer reg.Close()

	// Config and Policy accessors
	c := reg.Config()
	assert.Equal(t, cfg.BaselineCapacity, c.BaselineCapacity)

	p := reg.Policy()
	assert.NotZero(t, p.LoadSheddingThreshold)

	// Available tokens and admission
	avail := reg.AvailableTokens()
	assert.GreaterOrEqual(t, avail, 0.0)

	decision, waitDuration, err := reg.EvaluateAdmission(1)
	require.NoError(t, err)
	assert.Equal(t, AdmissionAllow, decision)
	assert.Zero(t, waitDuration)

	// Direct Admit
	admitted := reg.Admit(1)
	assert.True(t, admitted)

	// CalculateBackoff
	backoff := reg.CalculateBackoff(2)
	assert.Greater(t, backoff, time.Duration(0))
}

func TestAmbientCycle32_ConsensusArbiterElevation(t *testing.T) {
	arbiter := NewConsensusArbiter(WithMaxConsensusTasks(100))
	require.NotNil(t, arbiter)
	defer arbiter.Close()

	// Acquire lease
	lease, err := arbiter.AcquireLease("task-c32", "worker-1", 5*time.Second)
	require.NoError(t, err)
	require.NotNil(t, lease)

	// Clone lease
	clone := lease.Clone()
	assert.Equal(t, lease.TaskID, clone.TaskID)
	assert.Equal(t, lease.WorkerID, clone.WorkerID)

	// IsActive and IsExpired
	assert.True(t, lease.IsActive())
	assert.False(t, lease.IsExpired())

	// Renew lease
	renewed, renErr := arbiter.RenewLease("task-c32", "worker-1", 10*time.Second)
	require.NoError(t, renErr)
	assert.True(t, renewed.ExpiresAt.After(lease.ExpiresAt))

	// Commit task
	require.NoError(t, arbiter.CommitTask("task-c32", "worker-1"))
}

func TestAmbientCycle32_BranchHygieneDaemonElevation(t *testing.T) {
	pruner := &fakePruner{}
	daemon, err := NewBranchHygieneDaemon(
		pruner,
		WithHygieneInterval(5*time.Minute),
		WithHygieneDryRun(true),
		WithHygieneIdentity("test-hygiene-bot", "custom-hygiene", "custodian", "ambient"),
	)
	require.NoError(t, err)
	require.NotNil(t, daemon)

	assert.Equal(t, "custom-hygiene", daemon.Name())
	assert.Equal(t, "custodian", daemon.Role())
	assert.Equal(t, "ambient", daemon.Ecosystem())
	assert.Equal(t, "test-hygiene-bot", daemon.ID())

	// Exercise tick
	ctx := context.Background()
	daemon.Tick(ctx)
	st := daemon.Status()
	assert.NotEmpty(t, st)
}

func TestAmbientCycle32_WatcherDaemonLifecycleElevation(t *testing.T) {
	tempRoot := t.TempDir()
	stateDir := filepath.Join(tempRoot, paths.ProjectDataDir, paths.StateDir)
	require.NoError(t, fileutil.MkdirAll(stateDir, paths.DirPerm755))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Immediately cancelled

	err := RunWatcherDaemonWithEngine(ctx, tempRoot, nil, nil)
	assert.NoError(t, err)

	watcherCfg := DefaultWatcherConfig()
	err2 := RunWatcherDaemonWithEngineAndConfig(ctx, tempRoot, watcherCfg, nil, nil)
	assert.NoError(t, err2)
}

func TestAmbientCycle32_EpochSynchronizerElevation(t *testing.T) {
	// Checksum string representation
	algo := ChecksumSHA256
	assert.Equal(t, "sha256", algo.String())
	assert.True(t, algo.IsValid())
	assert.False(t, ChecksumAlgorithm("unknown").IsValid())

	// Checkpoint marshal / unmarshal
	cp := EpochCheckpoint{
		EpochID:   1,
		StateData: map[string]string{"key": "value"},
	}
	bytes, err := MarshalCheckpoint(cp)
	require.NoError(t, err)
	assert.NotEmpty(t, bytes)

	unmarshaled, uErr := UnmarshalCheckpoint(bytes)
	require.NoError(t, uErr)
	assert.Equal(t, uint64(1), unmarshaled.EpochID)
	assert.Equal(t, "value", unmarshaled.StateData["key"])

	// Empty unmarshal returns error
	_, emptyErr := UnmarshalCheckpoint([]byte{})
	require.Error(t, emptyErr)

	// Synchronizer with participants and checkpoint history
	sync := NewEpochSynchronizerWithParticipants([]string{"agent-alpha", "agent-beta"})
	require.NotNil(t, sync)
	defer sync.Close()

	committed, cErr := sync.CommitCheckpoint(1, bytes)
	require.NoError(t, cErr)
	assert.NotNil(t, committed)

	latest := sync.LatestCheckpoint()
	require.NotNil(t, latest)
	assert.Equal(t, uint64(1), latest.EpochID)

	history := sync.History()
	assert.Len(t, history, 1)
}

func TestAmbientCycle32_FaultIsolatorElevation(t *testing.T) {
	iso := DefaultFaultIsolator()
	require.NotNil(t, iso)

	// LastStateTransition and FailureCount
	ts := iso.LastStateTransition()
	assert.False(t, ts.IsZero())

	fails := iso.FailureCount()
	assert.GreaterOrEqual(t, fails, int64(0))

	tracker := iso.RateTracker()
	assert.NotNil(t, tracker)

	// SlidingWindowRateTracker ErrorRate and EvictStale
	rate := tracker.ErrorRate()
	assert.GreaterOrEqual(t, rate, 0.0)

	tracker.EvictStale(time.Now().UTC())
}

func TestAmbientCycle32_ConsensusArbiterExpiredCountAndLookup(t *testing.T) {
	arbiter := NewConsensusArbiter()
	require.NotNil(t, arbiter)
	defer arbiter.Close()

	// Initial expired count
	assert.Equal(t, 0, arbiter.ExpiredCount())

	// Propose task
	proposed, pErr := arbiter.ProposeTask("task-lookup-1", nil)
	require.NoError(t, pErr)
	assert.NotNil(t, proposed)

	// Lookup lease
	lease, err := arbiter.LookupLease("task-lookup-1")
	require.NoError(t, err)
	assert.Equal(t, "task-lookup-1", lease.TaskID)
	assert.Equal(t, ConsensusStatePending, lease.State)

	// Lookup nonexistent lease
	_, notFoundErr := arbiter.LookupLease("non-existent-task")
	require.Error(t, notFoundErr)
}
